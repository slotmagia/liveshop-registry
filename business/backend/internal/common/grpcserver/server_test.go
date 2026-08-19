package grpcserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformv1 "github.com/liveshop-platform/contracts/gen/go/platform/v1"
	"github.com/liveshop-platform/contracts/modulemanifest"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/biz"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/common/grpcauth"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/controlplane/provisioning"
	"github.com/lvtuopen-ai/liveshop-registry/backend/internal/data/memory"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

func TestPlatformGRPCServerPublishesActiveCapabilitiesOnlyToIdentity(t *testing.T) {
	certificateFiles, clientCredentials := testCertificates(t, "spiffe://liveshop.test/identity")
	releases := memory.NewReleaseRepository()
	manifest := grpcManifest()
	if _, err := releases.Register(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	if err := releases.Activate(context.Background(), nil, manifest.Metadata.ID, manifest.Metadata.Version); err != nil {
		t.Fatal(err)
	}
	surface := provisioning.New(provisioning.Config{Release: biz.NewRelease(releases)})
	server, err := New(Config{
		Address: "127.0.0.1:0",
		TLS:     certificateFiles,
		Workloads: []grpcauth.Workload{
			{
				SPIFFEID:    "spiffe://liveshop.test/identity",
				Subject:     "identity",
				Permissions: []string{"platform.registry.active-capabilities.read"},
			},
		},
	}, surface)
	if err != nil {
		t.Fatal(err)
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve()
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Stop(ctx); err != nil {
			t.Errorf("stop gRPC server: %v", err)
		}
		if err := <-serveErrors; err != nil {
			t.Errorf("serve gRPC: %v", err)
		}
	})

	connection, err := grpc.NewClient(server.Address(), grpc.WithTransportCredentials(clientCredentials))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	healthResponse, err := grpc_health_v1.NewHealthClient(connection).Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: platformv1.PlatformRegistryService_ServiceDesc.ServiceName,
	})
	if err != nil || healthResponse.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health response=%v err=%v", healthResponse, err)
	}

	client := platformv1.NewPlatformRegistryServiceClient(connection)
	catalog, err := client.GetActiveCapabilitySnapshot(ctx, &platformv1.GetActiveCapabilitySnapshotRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if catalog.GetRegistryRevision() != 2 || len(catalog.GetModules()) != 1 || catalog.GetModules()[0].GetReleaseVersion() != "1.0.0" {
		t.Fatalf("unexpected active capability snapshot: %#v", catalog)
	}
	module := catalog.GetModules()[0]
	if len(module.GetPermissions()) != 1 || len(module.GetHttpOperations()) != 1 || len(module.GetContributions()) != 1 || module.GetGrpc() == nil || module.GetBackend() == nil {
		t.Fatalf("active capability snapshot omitted authorization inputs: %#v", catalog.GetModules()[0])
	}
	contribution := module.GetContributions()[0]
	if contribution.GetArtifact() == nil || contribution.GetFrontend() == nil || len(contribution.GetFrontend().GetProps()) != 1 || len(contribution.GetFrontend().GetEvents()) != 1 || len(contribution.GetFrontend().GetActions()) != 1 || len(contribution.GetAllowedRoutes()) != 1 {
		t.Fatalf("active capability snapshot omitted contribution contract: %#v", contribution)
	}
	operation := module.GetHttpOperations()[0]
	if len(operation.GetRequestFields()) != 1 || len(operation.GetResponses()) != 1 || len(operation.GetResponses()[0].GetFields()) != 1 {
		t.Fatalf("active capability snapshot omitted HTTP field contract: %#v", operation)
	}
	method := module.GetGrpc().GetMethods()[0]
	if len(method.GetRequestFields()) != 1 || len(method.GetResponseFields()) != 1 {
		t.Fatalf("active capability snapshot omitted gRPC field contract: %#v", method)
	}
	if _, err := client.GetRouteSnapshot(ctx, &platformv1.GetRouteSnapshotRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("identity workload unexpectedly read gateway route snapshot: %v", err)
	}
}

func TestPlatformGRPCServerRejectsUntrustedSPIFFEIdentity(t *testing.T) {
	certificateFiles, clientCredentials := testCertificates(t, "spiffe://liveshop.test/untrusted")
	surface := provisioning.New(provisioning.Config{Release: biz.NewRelease(memory.NewReleaseRepository())})
	server, err := New(Config{
		Address: "127.0.0.1:0",
		TLS:     certificateFiles,
		Workloads: []grpcauth.Workload{
			{
				SPIFFEID:    "spiffe://liveshop.test/gateway",
				Subject:     "gateway",
				Permissions: []string{"platform.registry.routes.read"},
			},
		},
	}, surface)
	if err != nil {
		t.Fatal(err)
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
		<-serveErrors
	})
	connection, err := grpc.NewClient(server.Address(), grpc.WithTransportCredentials(clientCredentials))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = platformv1.NewPlatformRegistryServiceClient(connection).GetRouteSnapshot(ctx, &platformv1.GetRouteSnapshotRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("untrusted SPIFFE error=%v, want PermissionDenied", err)
	}
}

func grpcManifest() modulemanifest.Manifest {
	permission := "catalog.item.read"
	return modulemanifest.Manifest{
		APIVersion: modulemanifest.APIVersion,
		Kind:       modulemanifest.KindModuleRelease,
		Metadata: modulemanifest.Metadata{
			ID:      "catalog",
			Name:    "Catalog",
			Version: "1.0.0",
		},
		Spec: modulemanifest.Spec{
			Backend: modulemanifest.Backend{
				Service: "catalog",
				Origin:  "http://catalog:18090",
				HTTPRoutes: []modulemanifest.HTTPRoute{
					{
						Surface: "admin",
						Prefix:  "/admin/catalog",
						Operations: []modulemanifest.HTTPOperation{
							{
								ID:                  "catalog.item.list",
								Method:              "GET",
								Path:                "/admin/catalog/items",
								Summary:             "List items",
								Description:         "Lists catalog items.",
								Authentication:      "module-session",
								Idempotency:         "safe",
								RequiredPermissions: []string{permission},
								RequestFields:       []modulemanifest.CapabilityField{{Name: "status", Location: "query", Type: "string", Description: "Status filter"}},
								Responses: []modulemanifest.CapabilityResponse{
									{
										Status:      200,
										Description: "Item list",
										Fields:      []modulemanifest.CapabilityField{{Name: "items", Type: "array", Description: "Items"}},
									},
								},
							},
						},
					},
				},
				GRPC: &modulemanifest.GRPC{
					Service: "liveshop.catalog.v1.CatalogService", ContractVersion: "1.0.0", Endpoint: "dns:///catalog:19090", TransportSecurity: "tls1.3-mtls-spiffe",
					Methods: []modulemanifest.GRPCMethod{{Name: "GetItem", FullMethod: "/liveshop.catalog.v1.CatalogService/GetItem", Summary: "Get item", Description: "Gets one item.", Invocation: "unary", Idempotency: "safe", RecommendedDeadlineMS: 1000, RequiredPermissions: []string{permission}, RequestFields: []modulemanifest.CapabilityField{{Name: "id", Type: "string", Required: true, Description: "Item ID"}}, ResponseFields: []modulemanifest.CapabilityField{{Name: "name", Type: "string", Description: "Item name"}}}},
				},
			},
			Permissions: []modulemanifest.PermissionDefinition{
				{
					Code:     permission,
					Name:     "Read items",
					Resource: "catalog.item",
					Action:   "read",
				},
			},
			Contributions: []modulemanifest.Contribution{{
				ID: "catalog.admin.items", Surface: "admin", Kind: "page", Route: "/catalog/items", Title: "Items", Description: "Catalog items.", RequiredPermissions: []string{permission},
				AllowedRoutes: []modulemanifest.AllowedRoute{{Methods: []string{"GET"}, Prefix: "/admin/catalog", RequiredPermissions: []string{permission}}},
				Artifact:      modulemanifest.Artifact{Type: "iframe", Name: "@liveshop/catalog-admin", Version: "1.0.0", Entry: "http://catalog-ui", Integrity: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
				Frontend:      modulemanifest.FrontendContract{Component: "CatalogItems", Props: []modulemanifest.CapabilityField{{Name: "locale", Type: "string", Description: "Locale"}}, Events: []modulemanifest.FrontendEvent{{Name: "catalog.changed", Description: "Changed.", Payload: []modulemanifest.CapabilityField{{Name: "id", Type: "string", Description: "Item ID"}}}}, Actions: []modulemanifest.FrontendAction{{ID: "refresh", Label: "Refresh", Description: "Refresh items.", Invocation: "http", Target: "catalog.item.list", RequiredPermissions: []string{permission}}}},
			}},
		},
	}
}

func testCertificates(t *testing.T, clientSPIFFEID string) (TLSConfig, credentials.TransportCredentials) {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "LiveShop test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	serverCertificate, serverKey := signedCertificate(t, caCertificate, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	})
	spiffeURI, err := url.Parse(clientSPIFFEID)
	if err != nil {
		t.Fatal(err)
	}
	clientCertificate, clientKey := signedCertificate(t, caCertificate, caKey, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "test workload"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{spiffeURI},
	})
	directory := t.TempDir()
	caFile := filepath.Join(directory, "ca.pem")
	serverCertificateFile := filepath.Join(directory, "server.pem")
	serverKeyFile := filepath.Join(directory, "server-key.pem")
	writePEM(t, caFile, "CERTIFICATE", caDER)
	writePEM(t, serverCertificateFile, "CERTIFICATE", serverCertificate.Certificate[0])
	writeECKey(t, serverKeyFile, serverKey)
	roots := x509.NewCertPool()
	roots.AddCert(caCertificate)
	return TLSConfig{
			CertificateFile: serverCertificateFile,
			PrivateKeyFile:  serverKeyFile,
			ClientCAFile:    caFile,
		}, credentials.NewTLS(&tls.Config{
			MinVersion:   tls.VersionTLS13,
			RootCAs:      roots,
			Certificates: []tls.Certificate{clientCertificateWithKey(t, clientCertificate, clientKey)},
			ServerName:   "localhost",
		})
}

func signedCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, template *x509.Certificate) (tls.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}}, key
}

func clientCertificateWithKey(t *testing.T, certificate tls.Certificate, key *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	certificate.PrivateKey = key
	return certificate
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeECKey(t *testing.T, path string, key *ecdsa.PrivateKey) {
	t.Helper()
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, path, "EC PRIVATE KEY", der)
}
