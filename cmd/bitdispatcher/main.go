// Command bitdispatcher is the bitdispatcher computer-aided dispatch system: one binary with its web
// interface inside. Design: docs/DESIGN.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/biodream-llc/bitdispatcher/internal/advisory"
	"github.com/biodream-llc/bitdispatcher/internal/api"
	"github.com/biodream-llc/bitdispatcher/internal/auth"
	"github.com/biodream-llc/bitdispatcher/internal/bitvoice"
	"github.com/biodream-llc/bitdispatcher/internal/cad"
	"github.com/biodream-llc/bitdispatcher/internal/gis"
	"github.com/biodream-llc/bitdispatcher/internal/imagery"
	"github.com/biodream-llc/bitdispatcher/internal/mapdata"
	"github.com/biodream-llc/bitdispatcher/internal/web"
)

// version is set at build time by the Makefile (-X main.version=...).
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func defaultDataDir() string {
	if v := os.Getenv("BITDISPATCHER_DATA"); v != "" {
		return v
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "bitdispatcher")
	}
	return "data"
}

// run is main without the process exit, so it can be tested.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "reset-password" {
		return resetPassword(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("bitdispatcher", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "print the version and exit")
	addr := fs.String("addr", ":8443", "HTTPS address to serve the web interface and API on")
	dataDir := fs.String("data", defaultDataDir(), "data directory: ledger, users, integrations, certificate, recorded map (or BITDISPATCHER_DATA)")
	certFile := fs.String("tls-cert", "", "TLS certificate (PEM); empty uses a self-signed certificate kept in the data directory")
	keyFile := fs.String("tls-key", "", "TLS private key (PEM) for -tls-cert")
	redirect := fs.String("http-redirect", "", "also listen on this plain-HTTP address and redirect to HTTPS (such as :8080)")
	hooksAddr := fs.String("hooks-addr", "", "also take EIDO and radio pushes over plain HTTP on this loopback address (such as 127.0.0.1:9450), for a call-handling system on this computer that cannot trust the certificate")
	demo := fs.Bool("demo", false, "on a new system, add demonstration stations and units (labelled as demo data)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "bitdispatcher %s\n", version)
		return 0
	}
	logs := api.NewLogRing(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}), 500)
	log := slog.New(logs)
	if err := os.MkdirAll(*dataDir, 0o700); err != nil {
		log.Error("data directory", "err", err)
		return 1
	}
	eng, err := cad.Open(filepath.Join(*dataDir, "ledger.jsonl"))
	if err != nil {
		log.Error("ledger", "err", err)
		return 1
	}
	defer eng.Close()
	users, err := auth.Open(*dataDir)
	if err != nil {
		log.Error("users", "err", err)
		return 1
	}
	if users.Empty() {
		pw := auth.GeneratePassword()
		if err := users.Save("admin", "Administrator", auth.Admin, pw, false); err != nil {
			log.Error("first administrator", "err", err)
			return 1
		}
		eng.Record(cad.SystemActor, cad.EvUserSaved, cad.UserInfo{Username: "admin", Name: "Administrator", Role: auth.Admin})
		fmt.Fprintf(stderr, "\n  First start: sign in as  admin  with password  %s\n  It is shown once. Change it in Admin › Users.\n\n", pw)
	}
	if *demo {
		if err := seedDemo(eng); err != nil {
			log.Error("demo data", "err", err)
			return 1
		}
	}
	tlsCfg, fp, err := tlsConfig(*dataDir, *certFile, *keyFile, *addr)
	if err != nil {
		log.Error("tls", "err", err)
		return 1
	}
	var webH http.Handler
	if web.Available() {
		if webH, err = web.Handler(); err != nil {
			log.Error("web", "err", err)
			return 1
		}
	} else {
		log.Warn("the web interface was not built into this binary (make web); the API still works")
	}
	agent := "bitdispatcher/" + version + " (+https://github.com/biodream-llc/bitdispatcher)"
	photos, err := imagery.NewService(filepath.Join(*dataDir, "photos"), agent)
	if err != nil {
		log.Error("photos", "err", err)
		return 1
	}
	defer photos.Close()
	gisSvc, err := gis.NewService(filepath.Join(*dataDir, "gis"), agent, log)
	if err != nil {
		log.Error("gis", "err", err)
		return 1
	}
	var tilesTpl string
	eng.Read(func(s *cad.State) { tilesTpl = s.Settings.MapTiles })
	srv, err := api.New(&api.Server{Eng: eng, Auth: users, BV: bitvoice.New(eng, log), Log: log, DataDir: *dataDir, Version: version, Web: webH, Manual: web.Manual(),
		Tiles:  api.NewTileCache(filepath.Join(*dataDir, "tiles"), tilesTpl, version),
		Maps:   mapdata.NewService(filepath.Join(*dataDir, "map"), agent, log),
		Photos: photos, Advisory: advisory.New(eng, log), GIS: gisSvc, TLSFingerprint: fp, SelfSigned: *certFile == "", Logs: logs})
	if err != nil {
		log.Error("server", "err", err)
		return 1
	}
	defer srv.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv.Run(ctx)
	hs := &http.Server{Addr: *addr, Handler: srv, TLSConfig: tlsCfg, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn)}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Error("listen", "addr", *addr, "err", err)
		return 1
	}
	if *hooksAddr != "" {
		hl, err := hooksListener(*hooksAddr)
		if err != nil {
			log.Error("hooks", "addr", *hooksAddr, "err", err)
			return 1
		}
		hsrv := &http.Server{Handler: srv.HooksHandler(), ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10,
			ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelWarn)}
		go hsrv.Serve(hl)
		go func() { <-ctx.Done(); hsrv.Close() }()
		log.Info("taking EIDO and radio pushes over plain HTTP on loopback", "addr", "http://"+hl.Addr().String())
	}
	if *redirect != "" {
		go func() {
			rs := &http.Server{Addr: *redirect, ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				host, _, err := net.SplitHostPort(r.Host)
				if err != nil {
					host = r.Host
				}
				_, port, _ := net.SplitHostPort(ln.Addr().String())
				http.Redirect(w, r, "https://"+net.JoinHostPort(host, port)+r.URL.RequestURI(), http.StatusPermanentRedirect)
			})}
			if err := rs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Warn("http redirect", "err", err)
			}
		}()
	}
	log.Info("bitdispatcher "+version+" serving", "addr", "https://"+displayAddr(ln.Addr().String()), "data", *dataDir, "certificate-sha256", fp)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	if err := hs.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		return 1
	}
	return 0
}

func displayAddr(a string) string {
	host, port, err := net.SplitHostPort(a)
	if err != nil {
		return a
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}

// resetPassword sets a new generated password for a user, for an administrator locked out of the GUI.
func resetPassword(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dataDir := fs.String("data", defaultDataDir(), "data directory")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: bitdispatcher reset-password [-data dir] <username>")
		return 2
	}
	users, err := auth.Open(*dataDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	name := strings.ToLower(fs.Arg(0))
	var found *auth.Public
	for _, u := range users.List() {
		if u.Username == name {
			found = &u
		}
	}
	if found == nil {
		fmt.Fprintln(stderr, "no user", name)
		return 1
	}
	pw := auth.GeneratePassword()
	if err := users.Save(name, found.Name, found.Role, pw, false); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "New password for %s: %s\n(restart bitdispatcher if it is running, so it reads the change)\n", name, pw)
	return 0
}

// hooksListener listens on a loopback address only: plain HTTP never leaves this computer.
func hooksListener(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host)
	if host == "localhost" {
		ip = net.IPv4(127, 0, 0, 1)
	}
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("plain HTTP is only for this computer: use 127.0.0.1:PORT, not %s", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return ln, nil
}
