package Accounts

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"CitadelDesktop/Server/Runtime"
)

func transferControlRequest(o *Orchestrator, path, contentType, epoch, token string, body io.Reader) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/orchestrator/v1/handovers/"+path, body)
	request.Header.Set("Content-Type", contentType)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if epoch != "" {
		request.Header.Set(controlEpochHeader, epoch)
	}
	response := httptest.NewRecorder()
	o.Handler().ServeHTTP(response, request)
	return response
}

func TestProfileTransferRoutesRequireEnablementAuthenticationAndEpoch(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	raw, _ := json.Marshal(identity)
	for _, test := range []struct {
		enabled      bool
		token, epoch string
		want         int
	}{
		{false, testControlToken, "1", http.StatusNotFound},
		{true, "", "1", http.StatusUnauthorized},
		{true, testControlToken, "", http.StatusPreconditionRequired},
		{true, testControlToken, "01", http.StatusBadRequest},
		{true, testControlToken, "0", http.StatusBadRequest},
	} {
		o.handoverTransport = test.enabled
		response := transferControlRequest(o, "export", "application/json", test.epoch, test.token, bytes.NewReader(raw))
		if response.Code != test.want {
			t.Fatalf("status %d, want %d", response.Code, test.want)
		}
		if _, exists := s.Application("alpha"); !exists {
			t.Fatal("rejected transfer stopped source")
		}
		if _, exists := s.sourceFence("alpha"); exists {
			t.Fatal("rejected transfer persisted a source fence")
		}
	}
	if _, err := os.Stat(filepath.Join(s.config.DataRoot, "Accounts", controlFenceFile)); !os.IsNotExist(err) {
		t.Fatal("rejected request activated controller fence")
	}
}

func exportOverHTTP(t *testing.T, o *Orchestrator, identity Runtime.ProfileTransferIdentity) ProfileExport {
	t.Helper()
	o.handoverTransport = true
	raw, _ := json.Marshal(identity)
	response := transferControlRequest(o, "export", "application/json", "1", testControlToken, bytes.NewReader(raw))
	if response.Code != http.StatusOK {
		t.Fatalf("export status %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private export is cacheable")
	}
	var export ProfileExport
	if err := json.Unmarshal(response.Body.Bytes(), &export); err != nil {
		t.Fatal(err)
	}
	return export
}

func TestProfileTransferImmutableExportRetryAndDownload(t *testing.T) {
	s, _, o, identity, _ := handoverSourceFixture(t)
	app, _ := s.Application("alpha")
	original := app.DataDir
	export := exportOverHTTP(t, o, identity)
	if _, exists := s.Application("alpha"); exists {
		t.Fatal("exported source still running")
	}
	if err := os.WriteFile(filepath.Join(original, "later-private-file"), []byte("must not recapture"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := exportOverHTTP(t, o, identity); got != export {
		t.Fatal("retry recaptured modified source")
	}
	raw, _ := json.Marshal(export)
	response := transferControlRequest(o, "download", "application/json", "1", testControlToken, bytes.NewReader(raw))
	if response.Code != http.StatusOK || int64(response.Body.Len()) != export.ArchiveBytes || response.Header().Get("X-Citadel-Profile-SHA256") != export.Receipt.SHA256 {
		t.Fatal("download identity/length mismatch")
	}
	stage := filepath.Join(t.TempDir(), "restored")
	actual, err := Runtime.RestoreProfileArchive(t.Context(), bytes.NewReader(response.Body.Bytes()), stage, identity, export.Receipt.SHA256)
	if err != nil || actual != export.Receipt {
		t.Fatal("download is not the verified archive", err)
	}
	if _, err := os.Stat(filepath.Join(stage, "later-private-file")); !os.IsNotExist(err) {
		t.Fatal("immutable export included later state")
	}
	wrong := export
	wrong.ArchiveBytes++
	raw, _ = json.Marshal(wrong)
	response = transferControlRequest(o, "download", "application/json", "1", testControlToken, bytes.NewReader(raw))
	if response.Code != http.StatusConflict {
		t.Fatal("accepted different export receipt")
	}
	raw, _ = json.Marshal(export)
	response = transferControlRequest(o, "download", "application/json", "2", testControlToken, bytes.NewReader(raw))
	if response.Code != http.StatusOK {
		t.Fatal("new controller could not resume immutable download")
	}
	response = transferControlRequest(o, "download", "application/json", "1", testControlToken, bytes.NewReader(raw))
	if response.Code != http.StatusConflict {
		t.Fatal("old controller could still download")
	}
	archivePath := filepath.Join(s.config.DataRoot, "Transfers", "Exports", identity.OperationID, "profile.tar")
	if err := os.WriteFile(archivePath, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ExportSourceProfile(t.Context(), identity); err == nil {
		t.Fatal("corrupt immutable export was recaptured")
	}
}

func transferMultipart(t *testing.T, input ProfileRestoreRequest, archive []byte, extra bool) (string, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormField("metadata")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(part).Encode(input); err != nil {
		t.Fatal(err)
	}
	part, err = writer.CreateFormFile("archive", "ignored-name.tar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatal(err)
	}
	if extra {
		if _, err := writer.CreateFormField("extra"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return writer.FormDataContentType(), body.Bytes()
}

func TestProfileTransferRestoreAndSeparateActivation(t *testing.T) {
	_, _, receipt, archive, _ := stoppedAdoptionFixture(t)
	s, o := adoptionTarget(t)
	o.handoverTransport = true
	input := ProfileRestoreRequest{receipt, 5, adoptionSnapshot()}
	contentType, body := transferMultipart(t, input, archive, true)
	response := transferControlRequest(o, "restore", contentType, "1", testControlToken, bytes.NewReader(body))
	if response.Code != http.StatusBadRequest || !s.runtimeHandoverFenced("alpha") {
		t.Fatal("extra parts activated target")
	}
	contentType, body = transferMultipart(t, input, archive, false)
	response = transferControlRequest(o, "restore", contentType, "1", testControlToken, bytes.NewReader(body))
	if response.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", response.Code, response.Body.String())
	}
	var profile TargetProfile
	if err := json.Unmarshal(response.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.State != "restored" || !s.runtimeHandoverFenced("alpha") || len(s.AccountIDs()) != 0 {
		t.Fatal("restore started target")
	}
	raw, _ := json.Marshal(profile)
	response = transferControlRequest(o, "activate", "application/json", "1", testControlToken, bytes.NewReader(raw))
	if response.Code != http.StatusOK || len(s.AccountIDs()) != 0 || s.runtimeHandoverFenced("alpha") {
		t.Fatal("activation must publish pointer only")
	}
	response = transferControlRequest(o, "restore", "text/plain", "1", testControlToken, strings.NewReader("private invalid content"))
	if response.Code != http.StatusUnsupportedMediaType || strings.Contains(response.Body.String(), "private invalid") {
		t.Fatal("unsafe request error")
	}
}
