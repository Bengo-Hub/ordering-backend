package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bengo-Hub/httpware/objectstore"
	authclient "github.com/Bengo-Hub/shared-auth-client"
	"go.uber.org/zap"
)

func uploadRequest(t *testing.T, claims *authclient.Claims) *http.Request {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.NRGBA{255, 0, 0, 255})
	var pic bytes.Buffer
	_ = png.Encode(&pic, img)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "dish.png")
	_, _ = fw.Write(append(pic.Bytes(), []byte("<script>")...))
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/media/upload", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if claims != nil {
		r = r.WithContext(authclient.ContextWithClaims(r.Context(), claims))
	}
	return r
}

func TestMediaUpload(t *testing.T) {
	RegisterMediaTypes()
	root := t.TempDir()
	store, err := objectstore.New("ordering", objectstore.Config{LocalRoot: root, LocalURLBase: "http://localhost:4000"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewMediaHandler(zap.NewNop(), store)

	// No signed-in tenant user: refused (the route used to accept anonymous uploads).
	rec := httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous upload: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, &authclient.Claims{TenantID: "t1"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"key":"ordering/t1/menu-images/`) {
		t.Errorf("stored under the tenant's menu images: %s", rec.Body.String())
	}
	b, _, err := store.Get(t.Context(), extractKey(rec.Body.String()))
	if err != nil || bytes.Contains(b, []byte("<script>")) {
		t.Errorf("image must be stored re-encoded: %v", err)
	}

	r := uploadRequest(t, &authclient.Claims{TenantID: "t1"})
	r.URL.RawQuery = "type=kyc"
	rec = httptest.NewRecorder()
	h.Upload(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown type must be refused: %d", rec.Code)
	}
}

func extractKey(body string) string {
	i := strings.Index(body, `"key":"`)
	if i < 0 {
		return ""
	}
	rest := body[i+len(`"key":"`):]
	return rest[:strings.Index(rest, `"`)]
}
