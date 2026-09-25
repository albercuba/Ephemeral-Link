package web

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"

	"ephemeral-link/internal/config"
	sec "ephemeral-link/internal/crypto"
	"ephemeral-link/internal/redisstore"
	"ephemeral-link/internal/storage"
)

func newHandlerTestApp(t *testing.T) (*App, *redisstore.Store, *storage.Local, []byte) {
	t.Helper()
	redis := miniredis.RunT(t)
	store, err := redisstore.New("redis://" + redis.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	files, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	app := &App{cfg: config.Config{AppBaseURL: "https://links.example.test", EncryptionMasterKey: key}, store: store, files: files, log: slog.Default()}
	return app, store, files, key
}

func withRouteID(request *http.Request, id string) *http.Request {
	route := chi.NewRouteContext()
	route.URLParams.Add("id", id)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, route))
}

func TestRedirectCreatedStoresShortLivedReceipt(t *testing.T) {
	app, store, _, _ := newHandlerTestApp(t)
	request := httptest.NewRequest(http.MethodPost, "/secrets/text", nil)
	recorder := httptest.NewRecorder()
	item := redisstore.Item{ID: "receipt-item", CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}

	app.redirectCreated(recorder, request, "/s/receipt-item", item, "")
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/created" {
		t.Fatalf("redirect = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
	cookie := recorder.Result().Cookies()[0]
	if cookie.Name != createdReceiptCookieName || cookie.Value == "" || cookie.MaxAge <= 0 {
		t.Fatalf("unexpected receipt cookie: %#v", cookie)
	}
	receipt, err := store.GetCreatedReceipt(context.Background(), cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Link != "https://links.example.test/s/receipt-item" {
		t.Fatalf("receipt link = %q", receipt.Link)
	}
}

func TestDownloadFileClaimsOnceAndStreamsChunkedPayload(t *testing.T) {
	app, store, files, key := newHandlerTestApp(t)
	plain := bytes.Repeat([]byte("large streamed file\n"), 10000)
	dataKey, err := sec.NewDataKey()
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := sec.WrapKey(dataKey, key)
	if err != nil {
		t.Fatal(err)
	}
	itemID := "streamed-file"
	path, err := files.WriteStream(itemID, func(dst io.Writer) error {
		_, err := sec.EncryptReader(bytes.NewReader(plain), dst, dataKey, sec.DefaultChunkSize)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	item := redisstore.Item{ID: itemID, Type: "file", CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(), WrappedKeyNonce: wrapped.Nonce, WrappedKeyCiphertext: wrapped.Ciphertext, FileSize: int64(len(plain)), MimeType: "text/plain", SanitizedFilename: "stream.txt", StorageObjectPath: path}
	if err := store.Create(context.Background(), item, time.Hour); err != nil {
		t.Fatal(err)
	}

	request := withRouteID(httptest.NewRequest(http.MethodPost, "/f/"+itemID+"/download", nil), itemID)
	first := httptest.NewRecorder()
	app.downloadFile(first, request)
	if first.Code != http.StatusOK || first.Body.String() != string(plain) {
		t.Fatalf("first download status/body mismatch: status=%d bytes=%d", first.Code, first.Body.Len())
	}
	if got := first.Header().Get("Content-Length"); got == "" {
		t.Fatal("streamed download did not set Content-Length")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("claimed file still exists: %v", err)
	}

	second := httptest.NewRecorder()
	app.downloadFile(second, request)
	if second.Code != http.StatusSeeOther || second.Header().Get("Location") != "/expired" {
		t.Fatalf("second download = %d %q, want redirect to expired", second.Code, second.Header().Get("Location"))
	}
}
