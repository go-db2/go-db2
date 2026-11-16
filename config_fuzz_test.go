package db2

import (
	"net/url"
	"strings"
	"testing"
)

func FuzzParseDSN(f *testing.F) {
	f.Add("db2://db2inst1:SecretPass123@127.0.0.1:50000/TESTDB?ssl=false&allow_plaintext_auth=true&conn_timeout=10s")
	f.Add("db2s://appuser:SecretPass123@db2.example.com/PRODDB?block_size=131072")
	f.Add("host=localhost;port=50000;database=SAMPLE;user=u;password=p;ssl=false;allow_plaintext_auth=true;conn_timeout=15;")
	f.Add("db2://user:SecretPass123@")
	f.Add("db2://user:SecretPass123@?ssl=true")
	f.Add("db2://user:SecretPass123@a=b:badport")
	f.Add("db2://user:SecretPass123@a@b:badport")
	f.Add("")
	f.Add("localhost:50000/DB")

	const testSecret = "FuzzS3cr3tTok3n987!"

	f.Fuzz(func(t *testing.T, data string) {
		cfg, err := ParseDSN(data)
		if err == nil && cfg != nil {
			s := cfg.String()
			gs := cfg.GoString()
			if !strings.Contains(s, `Password:"******"`) || !strings.Contains(gs, `Password:"******"`) {
				t.Fatalf("Config String/GoString missing password redaction: %q / %q", s, gs)
			}
		}

		// Verify Config.String()/GoString() never leak a high-entropy password regardless of other fields.
		redactedCfg := Config{
			Host:     data,
			Port:     50000,
			Database: data,
			User:     data,
			Password: testSecret,
			UseSSL:   true,
		}
		if !strings.Contains(redactedCfg.String(), `Password:"******"`) || !strings.Contains(redactedCfg.GoString(), `Password:"******"`) {
			t.Fatalf("Config String/GoString missing Password:\"******\"")
		}
		if !strings.Contains(data, testSecret) {
			if strings.Contains(redactedCfg.String(), testSecret) || strings.Contains(redactedCfg.GoString(), testSecret) {
				t.Fatalf("Config String/GoString leaked testSecret")
			}
		}

		// Verify arbitrary URL suffixes do not leak testSecret in ParseDSN error messages or formatted Config.
		if !strings.ContainsAny(data, "\r\n") && !strings.Contains(data, "://") && !strings.Contains(data, testSecret) {
			pathUnesc, pathErr := url.PathUnescape(data)
			queryUnesc, queryErr := url.QueryUnescape(data)
			if (pathErr != nil || !strings.Contains(pathUnesc, testSecret)) &&
				(queryErr != nil || !strings.Contains(queryUnesc, testSecret)) {
				candCfg, candErr := ParseDSN("db2://user:" + testSecret + "@" + data)
				if candErr != nil {
					if strings.Contains(candErr.Error(), testSecret) {
						t.Fatalf("ParseDSN leaked testSecret for suffix %q: %v", data, candErr)
					}
				} else if candCfg != nil {
					if strings.Contains(candCfg.String(), testSecret) || strings.Contains(candCfg.GoString(), testSecret) {
						t.Fatalf("Config String/GoString leaked testSecret for suffix %q", data)
					}
				}
			}
		}
	})
}
