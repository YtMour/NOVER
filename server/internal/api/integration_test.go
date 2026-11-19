//go:build integration

// Integration coverage against a real PostgreSQL database. Run with:
//
//	TEST_DATABASE_URL=postgres://... go test -tags=integration ./internal/api
//
// The database should be disposable; the test creates synthetic products and
// users in the default tenant and removes them afterwards.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/YtMour/NOVER/server/internal/buildinfo"
	"github.com/YtMour/NOVER/server/internal/config"
	"github.com/YtMour/NOVER/server/internal/db"
	"github.com/YtMour/NOVER/server/internal/totp"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testClient struct {
	t     *testing.T
	base  string
	token string
}

func newIntegrationServer(t *testing.T) (*httptest.Server, *pgxpool.Pool, *testClient) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	if err := db.EnsureBootstrapIdentity(ctx, pool, "admin", "admin", "Integration Space"); err != nil {
		t.Fatalf("bootstrap identity: %v", err)
	}
	storageDir := os.Getenv("TEST_STORAGE_DIR")
	if storageDir == "" {
		storageDir = t.TempDir()
	}
	if err := os.MkdirAll(storageDir, 0o750); err != nil {
		t.Fatalf("create test storage dir: %v", err)
	}
	cfg := config.Config{
		Environment:        "development",
		DatabaseURL:        databaseURL,
		AdminUsername:      "admin",
		AdminPassword:      "admin",
		OperatorTokenKey:   []byte("integration-operator-token-key-0123456789"),
		RedeemHMACKey:      []byte("integration-redeem-hmac-key-0123456789"),
		AccessTokenKey:     []byte("integration-access-token-key-0123456789"),
		StorageDir:         storageDir,
		MaxUploadBytes:     1 << 20,
		AllowedOrigins:     []string{"http://localhost"},
		OperatorAPIPath:    "cp-7f3c",
		DefaultTenantName:  "Integration Space",
		DownloadTTLSeconds: 300,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(New(pool, cfg, logger).Handler())
	t.Cleanup(server.Close)
	return server, pool, &testClient{t: t, base: server.URL}
}

func (c *testClient) request(method, path string, body any, token string) (int, []byte) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatalf("build request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		c.t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		c.t.Fatalf("read response: %v", err)
	}
	return response.StatusCode, payload
}

func (c *testClient) requestJSON(method, path string, body any, token string, target any) int {
	c.t.Helper()
	status, payload := c.request(method, path, body, token)
	if target != nil {
		if err := json.Unmarshal(payload, target); err != nil {
			c.t.Fatalf("%s %s returned invalid JSON (%d): %s", method, path, status, payload)
		}
	}
	return status
}

type integrationLogin struct {
	AccessToken  string           `json:"access_token"`
	MFARequired  bool             `json:"mfa_required"`
	Session      operatorIdentity `json:"session"`
	MfaChallenge string           `json:"challenge_token"`
}

func (c *testClient) login(username, password string) integrationLogin {
	c.t.Helper()
	var login integrationLogin
	status := c.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/login", map[string]string{"username": username, "password": password}, "", &login)
	if status != http.StatusOK {
		c.t.Fatalf("login %s returned %d", username, status)
	}
	return login
}

func TestIntegrationDeliveryLifecycle(t *testing.T) {
	_, pool, client := newIntegrationServer(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()

	login := client.login("admin", "admin")
	if login.MFARequired {
		t.Fatal("integration database admin unexpectedly requires MFA; use a disposable database")
	}
	adminToken := login.AccessToken

	// --- product, release and delivery file --------------------------------
	var product struct {
		ID string `json:"id"`
	}
	status := client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/products", map[string]any{
		"name":           fmt.Sprintf("Integration product %d", suffix),
		"description":    "integration test product",
		"detail":         "integration detail",
		"highlights":     []string{"one", "two"},
		"license_terms":  "integration license",
		"requirements":   "Go and Postgres",
		"support_period": "30 days",
		"update_policy":  "LIFETIME",
		"update_until":   nil,
		"status":         "DRAFT",
	}, adminToken, &product)
	if status != http.StatusCreated {
		t.Fatalf("create product status = %d", status)
	}
	productID := product.ID
	t.Cleanup(func() { cleanupIntegrationProduct(ctx, t, pool, productID) })

	var release struct {
		ID string `json:"id"`
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/products/"+productID+"/releases", map[string]string{"version": "1.0.0", "changelog": "initial"}, adminToken, &release)
	if status != http.StatusCreated {
		t.Fatalf("create release status = %d", status)
	}

	fileContent := []byte("integration delivery payload " + fmt.Sprint(suffix))
	assetID := uploadIntegrationAsset(t, client, adminToken, productID, release.ID, "release.txt", fileContent)
	if assetID == "" {
		t.Fatal("asset upload did not return an id")
	}

	var productStatus struct {
		Status string `json:"status"`
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/products/"+productID+"/status", map[string]string{"status": "ACTIVE"}, adminToken, &productStatus)
	if status != http.StatusOK || productStatus.Status != "ACTIVE" {
		t.Fatalf("publish product status=%d body=%s", status, productStatus.Status)
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/products/"+productID+"/releases/"+release.ID+"/status", map[string]string{"status": "PUBLISHED"}, adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("publish release status = %d", status)
	}

	var detail struct {
		ID           string   `json:"id"`
		Highlights   []string `json:"highlights"`
		UpdatePolicy string   `json:"update_policy"`
	}
	status = client.requestJSON(http.MethodGet, "/api/v1/products/"+productID, nil, "", &detail)
	if status != http.StatusOK || detail.ID != productID || len(detail.Highlights) != 2 || detail.UpdatePolicy != "LIFETIME" {
		t.Fatalf("public detail status=%d payload=%#v", status, detail)
	}

	// --- issue, redeem, delivery, ticket, download -------------------------
	var issued struct {
		Codes []string `json:"codes"`
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/products/"+productID+"/codes", map[string]any{
		"count": 1, "code_ttl_days": 7, "access_days": 30, "max_downloads": 5, "release_id": release.ID,
	}, adminToken, &issued)
	if status != http.StatusCreated || len(issued.Codes) != 1 {
		t.Fatalf("issue codes status=%d body=%v", status, issued)
	}
	var redeemed struct {
		AccessToken string `json:"access_token"`
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/redeem", map[string]string{"code": issued.Codes[0]}, "", &redeemed)
	if status != http.StatusCreated || redeemed.AccessToken == "" {
		t.Fatalf("redeem status = %d", status)
	}
	buyerToken := redeemed.AccessToken
	var delivery struct {
		BaseVersion string `json:"base_version"`
		Assets      []struct {
			ID string `json:"id"`
		} `json:"assets"`
		MaxDownloads int `json:"max_downloads"`
	}
	status = client.requestJSON(http.MethodGet, "/api/v1/delivery", nil, buyerToken, &delivery)
	if status != http.StatusOK || delivery.BaseVersion != "1.0.0" || len(delivery.Assets) != 1 || delivery.MaxDownloads != 5 {
		t.Fatalf("delivery status=%d payload=%#v", status, delivery)
	}
	var ticket struct {
		URL string `json:"url"`
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/assets/"+delivery.Assets[0].ID+"/ticket", nil, buyerToken, &ticket)
	if status != http.StatusCreated || ticket.URL == "" {
		t.Fatalf("ticket status = %d", status)
	}
	downloadRequest, _ := http.NewRequest(http.MethodGet, client.base+ticket.URL, nil)
	downloadResponse, err := http.DefaultClient.Do(downloadRequest)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	downloaded, _ := io.ReadAll(downloadResponse.Body)
	downloadResponse.Body.Close()
	if downloadResponse.StatusCode != http.StatusOK || !bytes.Equal(downloaded, fileContent) {
		t.Fatalf("download status=%d bytes=%d want=%d", downloadResponse.StatusCode, len(downloaded), len(fileContent))
	}

	// --- refund revokes delivery ------------------------------------------
	var activity struct {
		Items []struct {
			CodeID    string `json:"code_id"`
			ProductID string `json:"product_id"`
		} `json:"items"`
	}
	status = client.requestJSON(http.MethodGet, "/api/v1/cp-7f3c/activity?limit=50", nil, adminToken, &activity)
	if status != http.StatusOK || len(activity.Items) == 0 {
		t.Fatalf("activity status=%d", status)
	}
	codeID := ""
	for _, item := range activity.Items {
		if item.ProductID == productID {
			codeID = item.CodeID
			break
		}
	}
	if codeID == "" {
		t.Fatal("activity did not include the integration product code")
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/codes/"+codeID+"/action", map[string]string{"action": "REFUND", "reason": "integration refund"}, adminToken, nil)
	if status != http.StatusOK {
		t.Fatalf("refund status = %d", status)
	}
	if status, _ := client.request(http.MethodGet, "/api/v1/delivery", nil, buyerToken); status != http.StatusUnauthorized {
		t.Fatalf("refunded delivery status = %d, want 401", status)
	}
}

func TestIntegrationInvitationMFAAndSystemStatus(t *testing.T) {
	_, pool, client := newIntegrationServer(t)
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("intuser%d", suffix)

	login := client.login("admin", "admin")
	if login.MFARequired {
		t.Fatal("integration database admin unexpectedly requires MFA; use a disposable database")
	}
	adminToken := login.AccessToken

	// --- system status + meta ---------------------------------------------
	var meta struct {
		Version       string `json:"version"`
		SchemaVersion string `json:"schema_version"`
	}
	status := client.requestJSON(http.MethodGet, "/api/v1/meta", nil, "", &meta)
	if status != http.StatusOK || meta.Version == "" || meta.SchemaVersion == "" {
		t.Fatalf("meta status=%d payload=%#v", status, meta)
	}
	var system struct {
		App struct {
			Version string `json:"version"`
		} `json:"app"`
		Database struct {
			Ready      bool `json:"ready"`
			Migrations struct {
				Applied int    `json:"applied"`
				Pending int    `json:"pending"`
				Latest  string `json:"latest"`
			} `json:"migrations"`
		} `json:"database"`
		Storage struct {
			Writable bool `json:"writable"`
		} `json:"storage"`
	}
	status = client.requestJSON(http.MethodGet, "/api/v1/cp-7f3c/system/status", nil, adminToken, &system)
	if status != http.StatusOK || !system.Database.Ready || system.Database.Migrations.Applied < 4 || system.Database.Migrations.Pending != 0 || system.App.Version != buildinfo.Version || !system.Storage.Writable {
		t.Fatalf("system status=%d payload=%#v", status, system)
	}

	// --- invitation accept ------------------------------------------------
	var invitation struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/members/invitations", map[string]string{
		"username": username, "display_name": "Integration User", "role": "EDITOR",
	}, adminToken, &invitation)
	if status != http.StatusCreated || invitation.Token == "" {
		t.Fatalf("create invitation status = %d", status)
	}
	var members struct {
		Items []struct {
			MembershipID string `json:"membership_id"`
			UserID       string `json:"user_id"`
			Username     string `json:"username"`
		} `json:"items"`
	}
	client.requestJSON(http.MethodGet, "/api/v1/cp-7f3c/members", nil, adminToken, &members)
	var userID string
	for _, item := range members.Items {
		if item.Username == username {
			userID = item.UserID
		}
	}
	t.Cleanup(func() { cleanupIntegrationUser(ctx, t, pool, userID, username) })

	passwordOne := "integration-password-1"
	status = client.requestJSON(http.MethodPost, "/api/v1/invitations/accept", map[string]string{"token": invitation.Token, "password": passwordOne}, "", nil)
	if status != http.StatusOK {
		t.Fatalf("accept invitation status = %d", status)
	}
	userLogin := client.login(username, passwordOne)
	userToken := userLogin.AccessToken

	// --- MFA setup, two-step login, disable --------------------------------
	var setup struct {
		Secret        string   `json:"secret"`
		RecoveryCodes []string `json:"recovery_codes"`
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/mfa/setup", nil, userToken, &setup)
	if status != http.StatusOK || setup.Secret == "" || len(setup.RecoveryCodes) != 10 {
		t.Fatalf("mfa setup status=%d", status)
	}
	code, err := totp.CodeAt(setup.Secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/mfa/enable", map[string]string{"code": code}, userToken, nil)
	if status != http.StatusOK {
		t.Fatalf("mfa enable status = %d", status)
	}
	stepOne := client.login(username, passwordOne)
	if !stepOne.MFARequired || stepOne.MfaChallenge == "" {
		t.Fatal("MFA-enabled login did not return a challenge")
	}
	secondCode, _ := totp.CodeAt(setup.Secret, time.Now())
	var verified integrationLogin
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/login/mfa", map[string]string{"challenge_token": stepOne.MfaChallenge, "code": secondCode}, "", &verified)
	if status != http.StatusOK || verified.AccessToken == "" {
		t.Fatalf("mfa login status = %d", status)
	}
	secondCode, _ = totp.CodeAt(setup.Secret, time.Now())
	status = client.requestJSON(http.MethodPost, "/api/v1/cp-7f3c/mfa/disable", map[string]string{"password": passwordOne, "code": secondCode}, verified.AccessToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("mfa disable status = %d", status)
	}

	// --- verification report surface --------------------------------------
	if info, err := os.Stat(os.Getenv("TEST_STORAGE_DIR")); err == nil && !info.IsDir() {
		t.Fatal("TEST_STORAGE_DIR should be a directory when set")
	}
}

func uploadIntegrationAsset(t *testing.T, client *testClient, token, productID, releaseID, fileName string, content []byte) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("release_id", releaseID); err != nil {
		t.Fatalf("write release field: %v", err)
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, client.base+"/api/v1/cp-7f3c/products/"+productID+"/assets", &body)
	if err != nil {
		t.Fatalf("build upload request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("upload asset: %v", err)
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("upload asset status=%d body=%s", response.StatusCode, payload)
	}
	var asset struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &asset); err != nil {
		t.Fatalf("decode asset: %v", err)
	}
	return asset.ID
}

func cleanupIntegrationProduct(ctx context.Context, t *testing.T, pool *pgxpool.Pool, productID string) {
	t.Helper()
	if productID == "" {
		return
	}
	statements := []string{
		`DELETE FROM download_events WHERE entitlement_id IN (SELECT id FROM entitlements WHERE product_id=$1) OR asset_id IN (SELECT id FROM assets WHERE product_id=$1)`,
		`DELETE FROM download_tickets WHERE entitlement_id IN (SELECT id FROM entitlements WHERE product_id=$1)`,
		`DELETE FROM admin_events WHERE resource_type='redeem_code' AND resource_id IN (SELECT id FROM redeem_codes WHERE product_id=$1)`,
		`DELETE FROM admin_events WHERE resource_id=$1 OR metadata_json->>'product_id'=$1`,
		`UPDATE redeem_codes SET entitlement_id=NULL WHERE product_id=$1`,
		`DELETE FROM entitlements WHERE product_id=$1`,
		`DELETE FROM redeem_codes WHERE product_id=$1`,
		`DELETE FROM assets WHERE product_id=$1`,
		`DELETE FROM product_releases WHERE product_id=$1`,
		`DELETE FROM product_images WHERE product_id=$1`,
		`DELETE FROM products WHERE id=$1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, productID); err != nil {
			t.Logf("cleanup statement failed: %v", err)
		}
	}
}

func cleanupIntegrationUser(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID, username string) {
	t.Helper()
	if userID == "" {
		_, _ = pool.Exec(ctx, `DELETE FROM operator_users WHERE LOWER(username)=LOWER($1)`, username)
		return
	}
	_, _ = pool.Exec(ctx, `DELETE FROM admin_events WHERE resource_id IN (SELECT id FROM operator_invitations WHERE user_id=$1) OR metadata_json->>'username'=$2`, userID, username)
	statements := []string{
		`DELETE FROM admin_events WHERE resource_id=$1`,
		`DELETE FROM operator_sessions WHERE user_id=$1`,
		`DELETE FROM operator_invitations WHERE user_id=$1`,
		`DELETE FROM tenant_memberships WHERE user_id=$1`,
		`DELETE FROM operator_users WHERE id=$1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, userID); err != nil {
			t.Logf("cleanup statement failed: %v", err)
		}
	}
}
