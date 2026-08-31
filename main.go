package main

import (
	"bufio"
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
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"

	"time"
)

var (
	appName        = "CSV/Excel Collector"
	appDescription = "Extracts data from uploaded files"
	version        = "0.11.0"
)

// StatusEvent is sent to the scheduler Unix socket
type StatusEvent struct {
	RunID     int    `json:"run_id"`
	Type      string `json:"type"` // "status" (default) or "audit"
	Component string `json:"component"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Progress  int    `json:"progress"`
}

// IPCClient is used to send events to the scheduler
type IPCClient struct {
	SocketPath string
	RunID      int
	Component  string
	Topic      string
	SourceName string
}

func (c *IPCClient) SendEvent(status, message string, progress int) {
	if c == nil || c.SocketPath == "" {
		return
	}
	conn, err := net.Dial("unix", c.SocketPath)
	if err != nil {
		log.Printf("[IPC ERROR] Failed to connect to scheduler socket: %v", err)
		return
	}
	defer conn.Close()

	if c.Topic != "" && c.SourceName != "" {
		message = fmt.Sprintf("%s: %s: %s", c.Topic, c.SourceName, message)
	}

	event := StatusEvent{
		RunID:    c.RunID,
		Type:     "status",
		Status:   status,
		Message:  message,
		Progress: progress,
	}
	data, _ := json.Marshal(event)
	_, _ = conn.Write(append(data, '\n'))
}

func (c *IPCClient) SendAudit(message string) {
	if c == nil || c.SocketPath == "" {
		return
	}
	conn, err := net.Dial("unix", c.SocketPath)
	if err != nil {
		log.Printf("[IPC ERROR] Failed to connect to scheduler socket: %v", err)
		return
	}
	defer conn.Close()

	if c.Topic != "" && c.SourceName != "" {
		message = fmt.Sprintf("%s: %s: %s", c.Topic, c.SourceName, message)
	}

	event := StatusEvent{
		RunID:     c.RunID,
		Type:      "audit",
		Component: c.Component,
		Message:   message,
	}
	data, _ := json.Marshal(event)
	_, _ = conn.Write(append(data, '\n'))
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
	// Fetch credentials via IPC if running under scheduler
	if dbCfg, masterKey, err := fetchCredentialsFromScheduler(); err == nil {
		if dbCfg != "" {
			os.Setenv("MITM_DB_CONFIG_JSON", dbCfg)
		}
		if masterKey != "" {
			os.Setenv("MASTER_KEY", masterKey)
		}
	} else if os.Getenv("RUN_ID") != "" && os.Getenv("SCHEDULER_SOCKET_PATH") != "" {
		log.Printf("[IPC Warning] Failed to get credentials from scheduler: %v", err)
	}

	version = strings.Split(version, "-")[0]

	var ipc *IPCClient
	socketPath := os.Getenv("SCHEDULER_SOCKET_PATH")
	runIDStr := os.Getenv("RUN_ID")
	if runIDStr != "" && socketPath != "" {
		runID, err := strconv.Atoi(runIDStr)
		if err == nil {
			ipc = &IPCClient{
				SocketPath: socketPath,
				RunID:      runID,
				Component:  "mitm_collector_csv-xls",
			}
		}
	}

	fatal := func(msg string, err error) {
		fullMsg := fmt.Sprintf("%s: %v", msg, err)
		if err == nil {
			fullMsg = msg
		}
		if ipc != nil {
			ipc.SendEvent("failed", fullMsg, 0)
			ipc.SendAudit("ERROR: " + fullMsg)
		}
		log.Fatalf("%s", fullMsg)
	}

	if len(os.Args) < 2 {
		fatal("Usage: mitm-collector-csv-xls <argsJSON>", nil)
	}

	argsJSON := os.Args[1]
	var args struct {
		File              string `json:"file"`
		Topic             string `json:"topic"`
		BusinessKeyColumn string `json:"business_key_column"`
		SourceName        string `json:"source_name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		fatal("Failed to parse args", err)
	}

	if args.File == "" || args.Topic == "" {
		fatal("Missing 'file' or 'topic' in args", nil)
	}

	if args.SourceName == "" {
		args.SourceName = "FILE_UPLOAD"
	}

	if ipc != nil {
		ipc.Topic = args.Topic
		ipc.SourceName = args.SourceName
	}

	defer os.Remove(args.File)

	ipc.SendEvent("processing", fmt.Sprintf("%s (%s) started. Processing file: %s", appName, version, args.File), 0)
	ipc.SendAudit(fmt.Sprintf("%s (%s) started", appName, version))

	configSource := "Environment Variables"
	dbConfigJSON := os.Getenv("MITM_DB_CONFIG_JSON")
	var dbCfg struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
		Database string `json:"database"`
	}

	if dbConfigJSON != "" {
		var fullCfg struct {
			DB struct {
				Host     string `json:"host"`
				Port     int    `json:"port"`
				User     string `json:"user"`
				Password string `json:"password"`
				Database string `json:"database"`
			} `json:"db"`
		}
		if err := json.Unmarshal([]byte(dbConfigJSON), &fullCfg); err != nil {
			fatal("Failed to parse MitM JSON configuration", err)
		}
		dbCfg.Host = fullCfg.DB.Host
		dbCfg.Port = fullCfg.DB.Port
		dbCfg.User = fullCfg.DB.User
		dbCfg.Password = fullCfg.DB.Password
		dbCfg.Database = fullCfg.DB.Database
		configSource = "JSON Config (MITM_DB_CONFIG_JSON)"
	} else {
		dbCfg.Host = os.Getenv("MITM_DB_HOST")
		if portStr := os.Getenv("MITM_DB_PORT"); portStr != "" {
			dbCfg.Port, _ = strconv.Atoi(portStr)
		}
		dbCfg.User = os.Getenv("MITM_DB_USER")
		dbCfg.Password = os.Getenv("MITM_DB_PASSWORD")
		dbCfg.Database = os.Getenv("MITM_DB_NAME")
	}

	ipc.SendAudit(fmt.Sprintf("Loaded database configuration from %s", configSource))

	sslMode := "disable"
	if os.Getenv("MITM_DB_SSLMODE") == "true" {
		sslMode = "require"
	}
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		dbCfg.User, dbCfg.Password, dbCfg.Host, dbCfg.Port, dbCfg.Database, sslMode)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	config_pool, err := pgxpool.ParseConfig(dsn)
	if err == nil {
		config_pool.MaxConns = 20
		config_pool.MaxConnIdleTime = 5 * time.Minute
		config_pool.MaxConnLifetime = 1 * time.Hour
	}
	var pool *pgxpool.Pool
	if err == nil {
		pool, err = pgxpool.NewWithConfig(ctx, config_pool)
	}
	if err != nil {
		ipc.SendEvent("failed", "DB connection failed", 0)
		log.Fatalf("DB connection failed: %v", err)
	}
	defer pool.Close()

	masterKey := os.Getenv("MASTER_KEY")
	if masterKey == "" {
		ipc.SendEvent("failed", "MASTER_KEY not set", 0)
		log.Fatal("MASTER_KEY environment variable is required")
	}
	kek, err := validateKEK(masterKey)
	if err != nil {
		if ipc != nil {
			ipc.SendEvent("failed", err.Error(), 0)
		}
		log.Fatalf("%v", err)
	}

	dek, err := generateRandomKey(32)
	if err != nil {
		ipc.SendEvent("failed", "Failed to generate DEK", 0)
		log.Fatalf("Failed to generate DEK: %v", err)
	}

	wrappedDEK, err := wrapKey(dek, kek)
	if err != nil {
		ipc.SendEvent("failed", "Failed to wrap DEK", 0)
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
		ipc.SendEvent("finished", "File empty or only headers", 100)
		return
	}

	headers := records[0]
	rows := records[1:]
	totalRows := len(rows)

	ipc.SendEvent("processing", fmt.Sprintf("Processing %d rows", totalRows), 10)

	
	recordsIngested := 0
	recordsFailed := 0

	batch := &pgx.Batch{}
	batchSize := 0
	const maxBatchSize = 1000

	executeBatch := func() {
		if batchSize == 0 {
			return
		}
		
		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Printf("Failed to begin transaction for batch: %v", err)
			recordsFailed += batchSize
			batch = &pgx.Batch{}
			batchSize = 0
			return
		}
		
		br := tx.SendBatch(ctx, batch)
		
		var batchError error
		for i := 0; i < batchSize; i++ {
			_, err := br.Exec()
			if err != nil {
				batchError = err
				break
			}
		}
		
		br.Close()
		
		if batchError != nil {
			tx.Rollback(ctx)
			log.Printf("Batch exec error: %v", batchError)
			recordsFailed += batchSize
		} else {
			if err := tx.Commit(ctx); err != nil {
				log.Printf("Failed to commit batch tx: %v", err)
				recordsFailed += batchSize
			} else {
				recordsIngested += batchSize
			}
		}
		
		batch = &pgx.Batch{}
		batchSize = 0
	}

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

		// Determine Business Key
		var businessKey string
		if args.BusinessKeyColumn != "" {
			if bkVal, ok := recordMap[args.BusinessKeyColumn]; ok && bkVal != "" {
				businessKey = bkVal
			} else {
				log.Printf("Missing business key for row %d, skipping", i)
				continue
			}
		} else {
			// Fallback: Use the first column if available
			if len(headers) > 0 && recordMap[headers[0]] != "" {
				businessKey = recordMap[headers[0]]
			} else {
				log.Printf("Missing business key for row %d, skipping", i)
				continue
			}
		}

		// Generate deterministic Correlation ID
		namespaceMitM := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
		correlationID := uuid.NewSHA1(namespaceMitM, []byte(businessKey))

		batch.Queue("INSERT INTO raw_ingestion (topic, source_system, correlation_id, payload, nonce, dek_id, status) VALUES ($1, 'CSV_UPLOAD', $2, $3, $4, $5, 'pending')",
			args.Topic, correlationID, encryptedPayload, nonce, keyID)
		batchSize++
		inserted++

		if batchSize >= maxBatchSize {
			executeBatch()
		}

		if inserted%100 == 0 || inserted == totalRows {
			progress := 10 + int(float64(inserted)/float64(totalRows)*90)
			ipc.SendEvent("processing", fmt.Sprintf("Processed %d/%d rows", inserted, totalRows), progress)
		}
	}

	executeBatch()

	ipc.SendAudit(fmt.Sprintf("Successfully ingested %d rows (Failed: %d) from file %s", recordsIngested, recordsFailed, args.File))
	ipc.SendAudit(fmt.Sprintf("%s (%s) finished", appName, version))
	ipc.SendEvent("finished", fmt.Sprintf("Ingestion complete. Topic: %s", args.Topic), 100)
}

func validateKEK(masterKey string) ([]byte, error) {
	var kek []byte
	if decoded, err := base64.StdEncoding.DecodeString(masterKey); err == nil {
		kek = decoded
	} else {
		kek = []byte(masterKey)
	}

	if len(kek) != 32 {
		return nil, fmt.Errorf("Invalid MASTER_KEY length: expected 32 bytes, got %d", len(kek))
	}
	return kek, nil
}

func fetchCredentialsFromScheduler() (string, string, error) {
	runIDStr := os.Getenv("RUN_ID")
	socketPath := os.Getenv("SCHEDULER_SOCKET_PATH")
	if runIDStr == "" || socketPath == "" {
		return "", "", fmt.Errorf("not running under scheduler")
	}
	
	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		return "", "", err
	}
	
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := map[string]interface{}{
		"type":   "get_credentials",
		"run_id": runID,
	}
	data, _ := json.Marshal(req)
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return "", "", err
	}

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		var resp struct {
			MasterKey    string `json:"master_key"`
			DBConfigJSON string `json:"db_config_json"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &resp); err == nil {
			return resp.DBConfigJSON, resp.MasterKey, nil
		}
	}
	return "", "", fmt.Errorf("no response or invalid JSON from scheduler")
}
