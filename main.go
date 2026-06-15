package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
)

var (
	appName        = "CSV/XLS Collector"
	appDescription = "Parses and ingests data from CSV and Excel files"
	version        = "1.0.0"
)

// Send IPC message
func sendStatus(socketPath string, runID int, status, message string, progress int) {
	if socketPath == "" || runID == 0 {
		return
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return
	}
	defer conn.Close()
	event := map[string]interface{}{
		"run_id":   runID,
		"status":   status,
		"message":  message,
		"progress": progress,
		"type":     "status",
	}
	_ = json.NewEncoder(conn).Encode(event)
}

func sendAudit(socketPath string, runID int, component, message string) {
	if socketPath == "" || runID == 0 {
		return
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return
	}
	defer conn.Close()
	event := map[string]interface{}{
		"run_id":    runID,
		"component": component,
		"message":   message,
		"type":      "audit",
	}
	_ = json.NewEncoder(conn).Encode(event)
}

// Crypto functions
func generateRandomKey(length int) ([]byte, error) {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptAESGCM(data, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	// Note: We are returning the ciphertext without prepending the nonce
	return aesgcm.Seal(nil, nonce, data, nil), nonce, nil
}

func wrapKey(dek, kek []byte) ([]byte, error) {
	ciphertext, nonce, err := encryptAESGCM(dek, kek)
	if err != nil {
		return nil, err
	}
	return append(nonce, ciphertext...), nil
}

func main() {
	socketPath := os.Getenv("SCHEDULER_SOCKET_PATH")
	runIDStr := os.Getenv("RUN_ID")
	runID, _ := strconv.Atoi(runIDStr)

	fatal := func(msg string, err error) {
		fullMsg := fmt.Sprintf("%s: %v", msg, err)
		if err == nil {
			fullMsg = msg
		}
		sendStatus(socketPath, runID, "FAILED", fullMsg, 0)
		sendAudit(socketPath, runID, "FileCollector", "ERROR: "+fullMsg)
		log.Fatalf(fullMsg)
	}

	if len(os.Args) < 2 {
		fatal("Usage: mitm-collector-csv-xls <argsJSON>", nil)
	}

	argsJSON := os.Args[1]
	var args struct {
		File  string `json:"file"`
		Topic string `json:"topic"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		fatal("Failed to parse args", err)
	}

	if args.File == "" || args.Topic == "" {
		fatal("Missing 'file' or 'topic' in args", nil)
	}

	defer os.Remove(args.File)

	sendStatus(socketPath, runID, "RUNNING", fmt.Sprintf("%s (%s) started. Processing file: %s", appName, version, args.File), 0)
	sendAudit(socketPath, runID, "RUNNING", fmt.Sprintf("%s (%s) started", appName, version))

	dbConfigJSON := os.Getenv("MITM_DB_CONFIG_JSON")
	var dbCfg struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Database string `json:"database"`
	}
	if err := json.Unmarshal([]byte(dbConfigJSON), &dbCfg); err != nil {
		sendStatus(socketPath, runID, "FAILED", "Failed to parse DB config", 0)
		log.Fatalf("Failed to parse DB config: %v", err)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		dbCfg.User, dbCfg.Password, dbCfg.Host, dbCfg.Port, dbCfg.Database)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		sendStatus(socketPath, runID, "FAILED", "DB connection failed", 0)
		log.Fatalf("DB connection failed: %v", err)
	}
	defer pool.Close()

	masterKey := os.Getenv("MASTER_KEY")
	if masterKey == "" {
		sendStatus(socketPath, runID, "FAILED", "MASTER_KEY not set", 0)
		log.Fatal("MASTER_KEY environment variable is required")
	}
	var kek []byte
	if decoded, err := base64.StdEncoding.DecodeString(masterKey); err == nil {
		kek = decoded
	} else {
		kek = []byte(masterKey)
	}

	// Adjust KEK to 32 bytes if necessary
	if len(kek) != 32 {
		adjusted := make([]byte, 32)
		copy(adjusted, kek)
		kek = adjusted
	}

	dek, err := generateRandomKey(32)
	if err != nil {
		sendStatus(socketPath, runID, "FAILED", "Failed to generate DEK", 0)
		log.Fatalf("Failed to generate DEK: %v", err)
	}

	wrappedDEK, err := wrapKey(dek, kek)
	if err != nil {
		sendStatus(socketPath, runID, "FAILED", "Failed to wrap DEK", 0)
		log.Fatalf("Failed to wrap DEK: %v", err)
	}

	var keyID string
	err = pool.QueryRow(ctx, "INSERT INTO storage_keys (wrapped_key, is_active) VALUES ($1, TRUE) RETURNING id", wrappedDEK).Scan(&keyID)
	if err != nil {
		fatal("Failed to store DEK", err)
	}

	var records [][]string
	ext := strings.ToLower(filepath.Ext(args.File))

	if ext == ".xlsx" {
		f, err := excelize.OpenFile(args.File)
		if err != nil {
			fatal("Failed to open XLSX file", err)
		}
		defer f.Close()

		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			fatal("XLSX file has no sheets", nil)
		}

		records, err = f.GetRows(sheets[0])
		if err != nil {
			fatal("Failed to read XLSX rows", err)
		}
	} else {
		file, err := os.Open(args.File)
		if err != nil {
			fatal("Failed to open file", err)
		}
		defer file.Close()

		reader := csv.NewReader(file)
		records, err = reader.ReadAll()
		if err != nil {
			fatal("Failed to read CSV", err)
		}
	}

	if len(records) < 2 {
		sendStatus(socketPath, runID, "SUCCESS", "File empty or only headers", 100)
		return
	}

	headers := records[0]
	rows := records[1:]
	totalRows := len(rows)

	sendStatus(socketPath, runID, "RUNNING", fmt.Sprintf("Processing %d rows", totalRows), 10)

	inserted := 0
	for i, row := range rows {
		recordMap := make(map[string]string)
		for j, val := range row {
			if j < len(headers) {
				recordMap[headers[j]] = val
			}
		}

		jsonData, _ := json.Marshal(recordMap)
		encryptedPayload, nonce, err := encryptAESGCM(jsonData, dek)
		if err != nil {
			log.Printf("Failed to encrypt row %d: %v", i, err)
			continue
		}

		_, err = pool.Exec(ctx, "INSERT INTO raw_ingestion (topic, source_system, payload, nonce, dek_id, status) VALUES ($1, 'CSV_UPLOAD', $2, $3, $4, 'pending')",
			args.Topic, encryptedPayload, nonce, keyID)
		if err != nil {
			log.Printf("Failed to insert row %d: %v", i, err)
			continue
		}
		inserted++

		if inserted%100 == 0 || inserted == totalRows {
			progress := 10 + int(float64(inserted)/float64(totalRows)*90)
			sendStatus(socketPath, runID, "RUNNING", fmt.Sprintf("Inserted %d/%d rows", inserted, totalRows), progress)
		}
	}

	sendAudit(socketPath, runID, "FileCollector", fmt.Sprintf("Successfully ingested %d rows from file %s", inserted, args.File))
	sendStatus(socketPath, runID, "SUCCESS", fmt.Sprintf("Ingestion complete. Topic: %s", args.Topic), 100)
}
