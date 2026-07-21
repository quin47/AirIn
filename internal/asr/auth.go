package asr

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	asrHost    = "openspeech.bytedance.com"
	asrPath    = "/api/v2/asr"
	asrRegion  = "cn-beijing"
	asrService = "openspeech"
)

func wsURL() string {
	return "wss://" + asrHost + asrPath
}

func signHeaders(accessKey, secretKey string) (http.Header, error) {
	now := time.Now().UTC()
	dateStr := now.Format("20060102T150405Z")
	datetimeStr := now.Format("20060102")

	credentialScope := fmt.Sprintf("%s/%s/%s/request", datetimeStr, asrRegion, asrService)

	signingStr := strings.Join([]string{
		dateStr,
		credentialScope,
	}, "\n")

	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(signingStr))
	signature := hex.EncodeToString(mac.Sum(nil))

	authHeader := fmt.Sprintf(
		"HMAC-SHA256 Credential=%s/%s, SignedHeaders=host;x-date, Signature=%s",
		accessKey, credentialScope, signature,
	)

	h := http.Header{}
	h.Set("Host", asrHost)
	h.Set("X-Date", dateStr)
	h.Set("Authorization", authHeader)
	return h, nil
}
