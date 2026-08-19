// Command grpccerts creates an ephemeral local mTLS trust bundle for Platform.
// Production deployments must supply certificates issued by their workload CA.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	output := flag.String("out", "./.run/grpc-certs", "certificate output directory")
	force := flag.Bool("force", false, "replace an existing local trust bundle")
	owner := flag.Int("owner", -1, "set the generated directory and files to this Unix uid")
	flag.Parse()
	if err := generate(*output, *force, *owner); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(output string, force bool, owner int) error {
	output, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if info, err := os.Stat(output); err == nil && info.IsDir() {
		entries, readErr := os.ReadDir(output)
		if readErr != nil {
			return readErr
		}
		if len(entries) > 0 && !force {
			if owner >= 0 {
				if err := setBundleOwner(output, os.Getuid(), os.Getgid()); err != nil {
					return fmt.Errorf("inspect existing local gRPC trust bundle: %w", err)
				}
			}
			if err := validateExistingBundle(output, time.Now()); err != nil {
				if owner >= 0 {
					if restoreErr := setBundleOwner(output, owner, owner); restoreErr != nil {
						return fmt.Errorf("restore local gRPC trust bundle owner after inspect: %v (bundle: %w)", restoreErr, err)
					}
				}
				if !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("existing local gRPC trust bundle is unusable; rotate certificates with -force and keep MySQL volumes: %w", err)
				}
				fmt.Fprintf(os.Stderr, "existing local gRPC trust bundle is missing required files (%v); rotating\n", err)
			} else {
				if owner >= 0 {
					if err := setBundleOwner(output, owner, owner); err != nil {
						return fmt.Errorf("restore local gRPC trust bundle owner: %w", err)
					}
				}
				fmt.Printf("reusing local Registry gRPC certificates in %s\n", output)
				return nil
			}
		}
	}
	if err := os.MkdirAll(output, 0o700); err != nil {
		return err
	}
	if err := reclaim(output, owner); err != nil {
		return err
	}
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "LiveShop local workload CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}
	if err := writePEM(filepath.Join(output, "ca.pem"), "CERTIFICATE", caDER); err != nil {
		return err
	}
	serverTemplate := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: "platform"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"platform", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	if err := issue(output, "server", serverTemplate, caCertificate, caKey); err != nil {
		return err
	}
	identityServer := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "identity"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"identity", "localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	if err := issue(output, "identity-server", identityServer, caCertificate, caKey); err != nil {
		return err
	}
	registryServer := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: "registry"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(365 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"registry", "localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	if err := issue(output, "registry-server", registryServer, caCertificate, caKey); err != nil {
		return err
	}
	for _, workload := range []struct {
		name     string
		spiffeID string
	}{
		{name: "gateway", spiffeID: "spiffe://liveshop.local/gateway"},
		{name: "release", spiffeID: "spiffe://liveshop.local/module-release-ci"},
		// Platform calls Identity's directory service as a workload client.
		{name: "platform-client", spiffeID: "spiffe://liveshop.local/platform"},
		{name: "identity-client", spiffeID: "spiffe://liveshop.local/identity"},
		// Catalog reads the Identity-owned subscription quota capability.
		{name: "catalog", spiffeID: "spiffe://liveshop.local/catalog"},
	} {
		identity, parseErr := url.Parse(workload.spiffeID)
		if parseErr != nil {
			return parseErr
		}
		template := &x509.Certificate{
			SerialNumber: serial(),
			Subject:      pkix.Name{CommonName: workload.name},
			NotBefore:    now.Add(-time.Minute),
			NotAfter:     now.Add(365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			URIs:         []*url.URL{identity},
		}
		if err := issue(output, workload.name, template, caCertificate, caKey); err != nil {
			return err
		}
	}
	if owner >= 0 {
		if err := setBundleOwner(output, owner, owner); err != nil {
			return fmt.Errorf("set local gRPC certificate owner: %w", err)
		}
	}
	fmt.Printf("local Registry gRPC certificates written to %s\n", output)
	return nil
}

func setBundleOwner(output string, uid, gid int) error {
	if err := os.Chown(output, uid, gid); err != nil {
		return err
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.Chown(filepath.Join(output, entry.Name()), uid, gid); err != nil {
			return err
		}
	}
	return nil
}

// validateExistingBundle makes the certificate job idempotent. Re-running a
// compose stack must not rotate the CA while Identity is already serving with
// the previous bundle in memory.
func validateExistingBundle(output string, now time.Time) error {
	for _, name := range []string{"ca.pem", "server.pem", "server-key.pem", "identity-server.pem", "identity-server-key.pem", "registry-server.pem", "registry-server-key.pem", "gateway.pem", "gateway-key.pem", "release.pem", "release-key.pem", "platform-client.pem", "platform-client-key.pem", "identity-client.pem", "identity-client-key.pem", "catalog.pem", "catalog-key.pem"} {
		payload, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if strings.HasSuffix(name, "-key.pem") {
			if block, _ := pem.Decode(payload); block == nil {
				return fmt.Errorf("%s is not PEM", name)
			}
			continue
		}
		block, _ := pem.Decode(payload)
		if block == nil {
			return fmt.Errorf("%s is not PEM", name)
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		if now.Before(certificate.NotBefore) || !now.Add(5*time.Minute).Before(certificate.NotAfter) {
			return fmt.Errorf("%s is expired or too close to expiry", name)
		}
	}
	return nil
}

// reclaim makes the output directory writable by this process again. A previous
// run handed it to owner, and the sandbox this runs in drops CAP_DAC_OVERRIDE,
// so being root is not sufficient on its own: ownership has to come back before
// the bundle can be replaced. Existing files are removed rather than truncated,
// because their mode is 0600 under the previous owner. Without this the bundle
// could be written exactly once per volume, and it expires after one year.
func reclaim(output string, owner int) error {
	if owner >= 0 {
		if err := os.Chown(output, os.Getuid(), os.Getgid()); err != nil {
			return fmt.Errorf("reclaim local gRPC certificate directory: %w", err)
		}
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(output, entry.Name())); err != nil {
			return fmt.Errorf("replace local gRPC trust bundle: %w", err)
		}
	}
	return nil
}

func issue(output, name string, template *x509.Certificate, ca *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	privateKeyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := writePEM(filepath.Join(output, name+".pem"), "CERTIFICATE", certificateDER); err != nil {
		return err
	}
	return writePEM(filepath.Join(output, name+"-key.pem"), "EC PRIVATE KEY", privateKeyDER)
}

func serial() *big.Int {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	value, err := rand.Int(rand.Reader, limit)
	if err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return value
}

func writePEM(path, blockType string, der []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600)
}
