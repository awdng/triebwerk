package transport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"sync"
	"time"
)

// Browsers accept a certificate by its hash only if it is valid for at
// most two weeks and has an ECDSA key.
const (
	selfSignedValidity = 13 * 24 * time.Hour
	// a server that runs for longer makes a new one in time
	selfSignedRenewal = 6 * 24 * time.Hour
)

// selfSignedCertificate makes a certificate nobody signed for the given
// host names and addresses
func selfSignedCertificate(hosts []string, now time.Time) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "triebwerk"},
		// clocks of clients are not exact
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(-time.Hour).Add(selfSignedValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, host := range hosts {
		if host == "" {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}

// ephemeralCertificate is a self signed certificate that is replaced before it expires
type ephemeralCertificate struct {
	hosts   []string
	mutex   sync.Mutex
	current *tls.Certificate
	made    time.Time
}

func newEphemeralCertificate(hosts []string) *ephemeralCertificate {
	return &ephemeralCertificate{hosts: hosts}
}

func (e *ephemeralCertificate) certificate(now time.Time) (*tls.Certificate, error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if e.current == nil || now.Sub(e.made) > selfSignedRenewal {
		cert, err := selfSignedCertificate(e.hosts, now)
		if err != nil {
			return nil, err
		}
		e.current, e.made = cert, now
	}
	return e.current, nil
}

// hash of the current certificate as browsers want it: base64 of its sha256
func (e *ephemeralCertificate) hash(now time.Time) string {
	cert, err := e.certificate(now)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(cert.Certificate[0])
	return base64.StdEncoding.EncodeToString(sum[:])
}
