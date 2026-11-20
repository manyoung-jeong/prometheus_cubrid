package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

var statPath = regexp.MustCompile(`^/v1/stat/([a-z]+)$`)

func main() {
	port := flag.Int("port", 9880, "listen port")
	source := flag.String("source", "mock", "data source: mock | statdump")
	dbsArg := flag.String("dbs", "testdb,demodb", "comma-separated database names")
	refreshMs := flag.Int("refresh", 5000, "snapshot refresh interval in milliseconds")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file (enables HTTPS when set with --tls-key)")
	tlsKey := flag.String("tls-key", "", "TLS private key file")
	flag.Parse()

	dbs := splitCSV(*dbsArg)
	if len(dbs) == 0 {
		log.Fatal("no databases given")
	}
	token := os.Getenv("CUBRID_API_TOKEN")

	schema, err := loadSchema()
	if err != nil {
		log.Fatal(err)
	}
	collector := NewCollector(*source, dbs, time.Duration(*refreshMs)*time.Millisecond).Start()

	authOK := func(r *http.Request) bool {
		if token == "" {
			return true
		}
		return r.Header.Get("Authorization") == "Bearer "+token
	}
	send := func(w http.ResponseWriter, code int, ctype, body string) {
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if path == "/healthz" {
			send(w, 200, "text/plain; charset=utf-8", "ok\n")
			return
		}
		if path == "/" {
			send(w, 200, "text/plain; charset=utf-8",
				"CUBRID Open API (v"+schema.Version+")\n"+
					"GET /metrics            OpenMetrics (Prometheus)\n"+
					"GET /v1/stat/all?db=    JSON (all groups)\n"+
					"GET /v1/stat/<group>    JSON (database|activity|io|lock|bgwriter|ha|space|availability)\n"+
					"GET /v1/databases       database list\n")
			return
		}
		if !authOK(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			send(w, 401, "text/plain; charset=utf-8", "unauthorized\n")
			return
		}

		snap := collector.SnapshotNow()
		if snap == nil {
			send(w, 503, "text/plain; charset=utf-8", "collector warming up\n")
			return
		}
		samples := buildSamples(schema, snap, time.Now())

		switch {
		case path == "/metrics":
			send(w, 200, "text/plain; version=0.0.4; charset=utf-8", renderOpenMetrics(schema, samples))
		case path == "/v1/databases":
			body, _ := json.MarshalIndent(map[string]interface{}{"databases": snap.Order}, "", "  ")
			send(w, 200, "application/json; charset=utf-8", string(body)+"\n")
		case statPath.MatchString(path):
			grp := statPath.FindStringSubmatch(path)[1]
			if grp == "all" {
				grp = ""
			}
			db := r.URL.Query().Get("db")
			body, _ := json.MarshalIndent(buildJson(schema, samples, snap, db, grp), "", "  ")
			send(w, 200, "application/json; charset=utf-8", string(body)+"\n")
		default:
			send(w, 404, "text/plain; charset=utf-8", "not found\n")
		}
	})

	addr := fmt.Sprintf(":%d", *port)
	scheme := "http"
	if *tlsCert != "" && *tlsKey != "" {
		scheme = "https"
	}
	log.Printf("CUBRID Open API (Go %s) listening on %s://0.0.0.0%s  source=%s  dbs=%s", version, scheme, addr, *source, strings.Join(dbs, ","))
	log.Printf("  /metrics  /v1/stat/all  /v1/databases  /healthz")
	srv := &http.Server{Addr: addr, Handler: mux}
	if scheme == "https" {
		log.Fatal(srv.ListenAndServeTLS(*tlsCert, *tlsKey))
	} else {
		log.Fatal(srv.ListenAndServe())
	}
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
