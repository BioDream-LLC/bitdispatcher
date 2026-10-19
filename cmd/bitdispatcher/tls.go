package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// tlsConfig loads the operator's certificate, or a self-signed one kept in the data directory (made on
// first start and again when it is within 30 days of expiry). It returns the certificate's SHA-256
// fingerprint, which BitVoice or a browser can be told to trust.
func tlsConfig(dataDir, certFile, keyFile, addr string) (*tls.Config, string, error) {
	if certFile == "" {
		dir := filepath.Join(dataDir, "tls")
		certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
		if needNew(certFile) {
			if err := selfSigned(dir, certFile, keyFile, addr); err != nil {
				return nil, "", err
			}
		}
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(cert.Certificate[0])
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}, hex.EncodeToString(sum[:]), nil
}

func needNew(certFile string) bool {
	b, err := os.ReadFile(certFile)
	if err != nil {
		return true
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return true
	}
	c, err := x509.ParseCertificate(blk.Bytes)
	return err != nil || time.Until(c.NotAfter) < 30*24*time.Hour
}

func selfSigned(dir, certFile, keyFile, addr string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	host, _ := os.Hostname()
	tpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "bitdispatcher", Organization: []string{"bitdispatcher (self-signed)"}},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{"localhost"},
	}
	if host != "" {
		tpl.DNSNames = append(tpl.DNSNames, host, host+".local")
	}
	tpl.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
	if ifs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range ifs {
			if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
				tpl.IPAddresses = append(tpl.IPAddresses, n.IP)
			}
		}
	}
	if h, _, err := net.SplitHostPort(addr); err == nil && h != "" {
		if ip := net.ParseIP(h); ip != nil && !ip.IsUnspecified() {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else if ip == nil {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}
