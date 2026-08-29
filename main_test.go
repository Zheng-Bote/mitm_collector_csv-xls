package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateKEK(t *testing.T) {
	validKey32 := strings.Repeat("!", 32)
	validKeyBase64 := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"Valid 32 byte string", validKey32, false},
		{"Valid 32 byte base64", validKeyBase64, false},
		{"Invalid short string", "too-short", true},
		{"Invalid long string", strings.Repeat("!", 33), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateKEK(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateKEK() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
