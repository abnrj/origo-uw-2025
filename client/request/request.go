package request

import (
	"bufio"
	tls "client/tls_fork"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	// "crypto/tls"
	"crypto/x509"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type RequestTLS struct {
	ServerDomain    string
	ServerPath      string
	ProxyURL        string
	UrlPrivateParts string
	AccessToken     string
	StorageLocation string
	Cookie          string
}

type RequestData struct {
	secrets   map[string][]byte
	recordMap map[string]tls.RecordMeta
}

func NewRequest() RequestTLS {
	return RequestTLS{
		ServerDomain:    "identity.uw.edu",
		ServerPath:      "/profile/api/profile/", // "testserver.origodata.io"
		ProxyURL:        "localhost:8082",
		UrlPrivateParts: "",
		AccessToken:     "",
		StorageLocation: "./local_storage/",
		Cookie:          "",
	}
}

func NewRequestWithCookie(c string) RequestTLS {
	return RequestTLS{
		ServerDomain:    "identity.uw.edu",
		ServerPath:      "/profile/api/profile/", // "testserver.origodata.io"
		ProxyURL:        "localhost:8082",
		UrlPrivateParts: "",
		AccessToken:     "",
		StorageLocation: "./local_storage/",
		Cookie:          c,
	}
}

func (r *RequestTLS) Store(data RequestData) error {
	jsonData := make(map[string]map[string]string)
	jsonData["keys"] = make(map[string]string)

	for k, v := range data.secrets {
		jsonData["keys"][k] = hex.EncodeToString(v)
	}
	for k, v := range data.recordMap {
		jsonData[k] = make(map[string]string)
		jsonData[k]["typ"] = v.Typ
		jsonData[k]["additionalData"] = hex.EncodeToString(v.AdditionalData)
		jsonData[k]["payload"] = hex.EncodeToString(v.Payload)
		jsonData[k]["ciphertext"] = hex.EncodeToString(v.Ciphertext)
	}

	file, err := json.MarshalIndent(jsonData, "", " ")
	if err != nil {
		log.Error().Err(err).Msg("json.MarshalIndent")
		return err
	}
	err = os.WriteFile(r.StorageLocation+"session_params_13.json", file, 0644)
	if err != nil {
		log.Error().Err(err).Msg("os.WriteFile")
	}
	return err
}

func (r *RequestTLS) Call(hsOnly bool) (RequestData, error) {

	// tls configs
	config := &tls.Config{
		InsecureSkipVerify:       false,
		CurvePreferences:         []tls.CurveID{tls.CurveP256},
		PreferServerCipherSuites: false,
		MinVersion:               tls.VersionTLS13,
		MaxVersion:               tls.VersionTLS13,
		CipherSuites:             []uint16{
			// tls.TLS_AES_128_GCM_SHA256,
		},
		ServerName: r.ServerDomain,
	}

	// local server testing settings
	if r.ServerDomain == "localhost" {
		PathCaCrt := "../certs/certificates/ca.crt"

		// set up cert verification
		caCert, _ := os.ReadFile(PathCaCrt)
		caCertPool, _ := x509.SystemCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		config.RootCAs = caCertPool

		r.ServerDomain += ":8081"
	}

	fmt.Printf("#rq.go 101: %v\n", config.CipherSuites)

	// measure start time
	start := time.Now()

	// tls connection
	conn, err := tls.Dial("tcp", r.ProxyURL, config)
	fmt.Printf("#rq.go 108: %v\n", config.CipherSuites)
	if err != nil {
		log.Error().Err(err).Msg("tls.Dial()")
		return RequestData{}, err
	}
	defer conn.Close()
	// Get connection state after handshake
	state := conn.ConnectionState()
	cipherSuiteName := tls.CipherSuiteName(state.CipherSuite)

	log.Info().
		Str("cipher_suite", cipherSuiteName).
		Str("cipher_id", fmt.Sprintf("0x%04X", state.CipherSuite)).
		Str("tls_version", fmt.Sprintf("0x%04X", state.Version)).
		Msg("TLS connection established")

	// tls handshake time
	elapsed := time.Since(start)
	log.Debug().Str("time", elapsed.String()).Msg("client tls handshake took.")
	// state := conn.ConnectionState()

	// return here if handshakeOnly flag set
	if hsOnly {
		return RequestData{}, nil
	}

	// server settings
	serverURL := "https://" + r.ServerDomain + r.ServerPath
	if r.UrlPrivateParts != "" {
		serverURL += r.UrlPrivateParts
	}

	// measure request-response roundtrip
	start = time.Now()

	// build request
	request, _ := http.NewRequest(http.MethodGet, serverURL, nil)
	request.Close = false

	// request headers
	request.Header.Set("Content-Type", "application/json")
	if r.AccessToken != "" {
		request.Header.Set("Authorization", "Bearer "+r.AccessToken)
	}
	if r.Cookie != "" {
		request.Header.Set("Cookie", "identityuwsession="+r.Cookie)
	}

	// initialize connection buffers
	bufr := bufio.NewReader(conn)
	bufw := bufio.NewWriter(conn)

	// write request to connection buffer
	err = request.Write(bufw)
	if err != nil {
		log.Error().Err(err).Msg("request.Write(bufw)")
		return RequestData{}, err
	}

	// writes buffer data into connection io.Writer
	err = bufw.Flush()
	if err != nil {
		log.Error().Err(err).Msg("bufw.Flush()")
		return RequestData{}, err
	}

	// read response
	resp, err := http.ReadResponse(bufr, request)
	if err != nil {
		log.Error().Err(err).Msg("http.ReadResponse(bufr, request)")
		return RequestData{}, err
	}
	defer resp.Body.Close()

	// reads response body
	msg, _ := io.ReadAll(resp.Body)
	log.Info().Msg("response data:")
	log.Info().Msg(string(msg))

	// catch time
	elapsed = time.Since(start)
	log.Debug().Str("time", elapsed.String()).Msg("client request-response roundtrip took.")

	// access to recorded session data
	return RequestData{
		secrets:   conn.GetSecretMap(),
		recordMap: conn.GetRecordMap(),
	}, nil
}

func (r *RequestTLS) Call2(hsOnly bool) (RequestData, error) {

	// tls configs
	config := &tls.Config{
		InsecureSkipVerify:       false,
		CurvePreferences:         []tls.CurveID{tls.CurveP256},
		PreferServerCipherSuites: false,
		// MinVersion:               tls.VersionTLS13,
		// MaxVersion:               tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		// ServerName: r.ServerDomain,
	}

	// local server testing settings
	if r.ServerDomain == "localhost" {
		PathCaCrt := "../certs/certificates/ca.crt"

		// set up cert verification
		caCert, _ := os.ReadFile(PathCaCrt)
		caCertPool, _ := x509.SystemCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		config.RootCAs = caCertPool

		r.ServerDomain += ":8081"
	}

	// measure start time
	start := time.Now()

	// tls connection
	conn, err := tls.Dial("tcp", "localhost:8081", config)
	if err != nil {
		log.Error().Err(err).Msg("tls.Dial()")
		return RequestData{}, err
	}
	defer conn.Close()

	// tls handshake time
	elapsed := time.Since(start)
	log.Debug().Str("time", elapsed.String()).Msg("client tls handshake took.")
	// state := conn.ConnectionState()

	// return here if handshakeOnly flag set
	if hsOnly {
		return RequestData{}, nil
	}

	// server settings
	serverURL := "https://" + r.ServerDomain + r.ServerPath
	if r.UrlPrivateParts != "" {
		serverURL += r.UrlPrivateParts
	}

	// measure request-response roundtrip
	start = time.Now()

	// build request
	request, _ := http.NewRequest(http.MethodGet, serverURL, nil)
	request.Close = false

	// request headers
	request.Header.Set("Content-Type", "application/json")
	if r.AccessToken != "" {
		request.Header.Set("Authorization", "Bearer "+r.AccessToken)
	}

	// initialize connection buffers
	bufr := bufio.NewReader(conn)
	bufw := bufio.NewWriter(conn)

	// write request to connection buffer
	err = request.Write(bufw)
	if err != nil {
		log.Error().Err(err).Msg("request.Write(bufw)")
		return RequestData{}, err
	}

	// writes buffer data into connection io.Writer
	err = bufw.Flush()
	if err != nil {
		log.Error().Err(err).Msg("bufw.Flush()")
		return RequestData{}, err
	}

	// read response
	resp, err := http.ReadResponse(bufr, request)
	if err != nil {
		log.Error().Err(err).Msg("http.ReadResponse(bufr, request)")
		return RequestData{}, err
	}
	defer resp.Body.Close()

	// reads response body
	msg, _ := io.ReadAll(resp.Body)
	log.Info().Msg("response data:")
	log.Info().Msg(string(msg))
	sizeStr := strconv.Itoa(len(msg))
	log.Info().Msg(sizeStr)

	// catch time
	elapsed = time.Since(start)
	log.Debug().Str("time", elapsed.String()).Msg("client request-response roundtrip took.")

	// access to recorded session data
	return RequestData{
		secrets:   conn.GetSecretMap(),
		recordMap: conn.GetRecordMap(),
	}, nil
}
