package Runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func transferFixture(t *testing.T) (string, ProfileTransferIdentity, map[string][]byte) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("hosted profile capture requires Unix link-count metadata")
	}
	directory := t.TempDir()
	identity := ProfileTransferIdentity{OperationID: "operation-one", AccountID: "account-one", RuntimeID: "runtime-one", TenantID: "tenant-one", SourceCellID: "stable-cell", SourceEpoch: 4, ConfigurationRevision: 39, ConfigurationDigest: strings.Repeat("a", 64)}
	files := map[string][]byte{
		"Runtime/ProfileID":        []byte("0fd2da8e-72d0-4c6c-9bd3-574963f20142\n"),
		"Operations.sqlite":        {0, 255, 1, 3, 8, 9},
		"Configuration/state.json": []byte(`{"revision":39,"unknown.future":{"preserve":true}}`),
		"State/safety.json":        []byte(`{"lane":"food","expiresAt":"2026-09-14T00:00:00Z"}`),
		"Session/retry.json":       []byte(`{"retryAt":"2026-09-14T01:00:00Z","pending":"uncertain"}`),
	}
	for name, value := range files {
		full := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, value, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(directory, "EmptyHistory"), 0700); err != nil {
		t.Fatal(err)
	}
	return directory, identity, files
}

func captureFixture(t *testing.T) ([]byte, ProfileArchiveReceipt, map[string][]byte) {
	t.Helper()
	directory, identity, files := transferFixture(t)
	var buffer bytes.Buffer
	receipt, err := WriteProfileArchive(t.Context(), directory, &buffer, identity)
	if err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), receipt, files
}

func TestProfileArchiveRoundTripPreservesAllState(t *testing.T) {
	raw, receipt, files := captureFixture(t)
	destination := filepath.Join(t.TempDir(), "restored")
	restored, err := RestoreProfileArchive(t.Context(), bytes.NewReader(raw), destination, receipt.Identity, receipt.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if restored != receipt || restored.Files != len(files) {
		t.Fatalf("receipt mismatch: %v / %v", restored, receipt)
	}
	for name, want := range files {
		full := filepath.Join(destination, filepath.FromSlash(name))
		got, err := os.ReadFile(full)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s changed: %v", name, err)
		}
		info, err := os.Stat(full)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("unsafe mode: %s", name)
		}
	}
	if info, err := os.Stat(filepath.Join(destination, "EmptyHistory")); err != nil || !info.IsDir() {
		t.Fatal("empty directory lost")
	}
	if _, err := os.Stat(filepath.Join(destination, "Runtime/Profile.lock")); !os.IsNotExist(err) {
		t.Fatal("copied live lease")
	}
	lease, err := AcquireProfileLease(destination)
	if err != nil {
		t.Fatal(err)
	}
	if lease.ProfileID != receipt.ProfileID {
		t.Fatal("profile identity changed")
	}
	lease.Close()
}

func TestProfileArchiveRefusesActiveProfile(t *testing.T) {
	directory, identity, _ := transferFixture(t)
	lease, err := AcquireProfileLease(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	var buffer bytes.Buffer
	_, err = WriteProfileArchive(t.Context(), directory, &buffer, identity)
	if !errors.Is(err, ErrProfileInUse) || buffer.Len() != 0 {
		t.Fatalf("active profile captured: %v", err)
	}
}

func TestProfileArchiveRejectsSourceLinks(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "lease-symlink", "runtime-symlink"} {
		t.Run(kind, func(t *testing.T) {
			directory, identity, _ := transferFixture(t)
			outside := filepath.Join(t.TempDir(), "private")
			if err := os.WriteFile(outside, []byte("outside-profile"), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(outside, filepath.Join(directory, "link"))
			case "hardlink":
				err = os.Link(outside, filepath.Join(directory, "link"))
			case "lease-symlink":
				err = os.Symlink(outside, filepath.Join(directory, "Runtime/Profile.lock"))
			case "runtime-symlink":
				if err = os.Rename(filepath.Join(directory, "Runtime"), filepath.Join(directory, "RealRuntime")); err == nil {
					err = os.Symlink(filepath.Join(directory, "RealRuntime"), filepath.Join(directory, "Runtime"))
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			var buffer bytes.Buffer
			if _, err = WriteProfileArchive(t.Context(), directory, &buffer, identity); err == nil {
				t.Fatal("unsafe link captured")
			}
			if buffer.Len() != 0 {
				t.Fatal("capture exposed data before validation completed")
			}
		})
	}
}

type archiveMutatingWriter struct {
	bytes.Buffer
	mutate func()
}

func (writer *archiveMutatingWriter) Write(value []byte) (int, error) {
	if writer.mutate != nil {
		fn := writer.mutate
		writer.mutate = nil
		fn()
	}
	return writer.Buffer.Write(value)
}

func TestProfileArchiveDetectsSourceMutation(t *testing.T) {
	directory, identity, _ := transferFixture(t)
	writer := &archiveMutatingWriter{mutate: func() {
		if err := os.WriteFile(filepath.Join(directory, "Operations.sqlite"), []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	if _, err := WriteProfileArchive(t.Context(), directory, writer, identity); err == nil {
		t.Fatal("changed profile accepted")
	}
}

func TestProfileArchiveNeverOverwritesDestination(t *testing.T) {
	raw, receipt, _ := captureFixture(t)
	destination := t.TempDir()
	sentinel := filepath.Join(destination, "keep")
	if err := os.WriteFile(sentinel, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreProfileArchive(t.Context(), bytes.NewReader(raw), destination, receipt.Identity, receipt.SHA256); err == nil {
		t.Fatal("existing directory overwritten")
	}
	if value, err := os.ReadFile(sentinel); err != nil || string(value) != "original" {
		t.Fatal("existing data removed")
	}
}

func TestProfileArchiveRejectsCorruptionAndWrongIdentity(t *testing.T) {
	raw, receipt, _ := captureFixture(t)
	for _, kind := range []string{"checksum", "operation", "epoch", "configuration", "corruption", "truncation", "trailing-data", "cancellation"} {
		t.Run(kind, func(t *testing.T) {
			input := append([]byte(nil), raw...)
			expected := receipt.Identity
			digest := receipt.SHA256
			ctx := t.Context()
			switch kind {
			case "checksum":
				digest = strings.Repeat("0", 64)
			case "operation":
				expected.OperationID = "other-operation"
			case "epoch":
				expected.SourceEpoch++
			case "configuration":
				expected.ConfigurationRevision++
			case "corruption":
				input[len(input)/2] ^= 1
			case "truncation":
				input = input[:len(input)/2]
			case "trailing-data":
				input = append(input, []byte("extra archive")...)
				sum := sha256.Sum256(input)
				digest = hex.EncodeToString(sum[:])
			case "cancellation":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			destination := filepath.Join(t.TempDir(), "stage")
			if _, err := RestoreProfileArchive(ctx, bytes.NewReader(input), destination, expected, digest); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatal("failed restore left an adoptable profile")
			}
		})
	}
}

func maliciousArchive(t *testing.T, change func(*profileArchiveDocument), kind byte) ([]byte, ProfileTransferIdentity, string) {
	t.Helper()
	_, identity, _ := transferFixture(t)
	content := []byte("profile-id\n")
	sum := sha256.Sum256(content)
	document := profileArchiveDocument{SchemaVersion: 1, Identity: identity, ProfileID: "profile-id", Entries: []profileArchiveEntry{
		{Name: "Runtime", Directory: true}, {Name: "Runtime/ProfileID", Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])},
	}}
	change(&document)
	manifest, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{Name: profileArchiveManifest, Size: int64(len(manifest)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	writer.Write(manifest)
	for _, item := range document.Entries {
		header := &tar.Header{Name: item.Name, Size: item.Size, Mode: 0600, Typeflag: tar.TypeReg}
		if item.Directory {
			header.Typeflag = tar.TypeDir
		} else if kind != 0 {
			header.Typeflag = kind
			header.Linkname = "/outside"
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if !item.Directory && kind == 0 && item.Size > 0 {
			writer.Write(content)
		}
	}
	writer.Close()
	sum = sha256.Sum256(buffer.Bytes())
	return buffer.Bytes(), identity, hex.EncodeToString(sum[:])
}

func TestProfileArchiveRejectsUnsafeManifestAndEntries(t *testing.T) {
	cases := map[string]func(*profileArchiveDocument){
		"traversal":        func(d *profileArchiveDocument) { d.Entries[1].Name = "../outside" },
		"absolute":         func(d *profileArchiveDocument) { d.Entries[1].Name = "/outside" },
		"backslash":        func(d *profileArchiveDocument) { d.Entries[1].Name = "Runtime\\outside" },
		"alternate-stream": func(d *profileArchiveDocument) { d.Entries[1].Name = "Runtime/id:stream" },
		"duplicate":        func(d *profileArchiveDocument) { d.Entries = append(d.Entries, d.Entries[1]) },
		"schema":           func(d *profileArchiveDocument) { d.SchemaVersion = 2 },
		"missing-identity": func(d *profileArchiveDocument) { d.Entries[1].Name = "Runtime/OtherID" },
		"wrong-profile-id": func(d *profileArchiveDocument) { d.ProfileID = "different" },
		"lease":            func(d *profileArchiveDocument) { d.Entries[1].Name = "Runtime/Profile.lock" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			raw, identity, digest := maliciousArchive(t, change, 0)
			destination := filepath.Join(t.TempDir(), "stage")
			if _, err := RestoreProfileArchive(t.Context(), bytes.NewReader(raw), destination, identity, digest); err == nil {
				t.Fatal("unsafe manifest accepted")
			}
		})
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo} {
		t.Run(string(kind), func(t *testing.T) {
			raw, identity, digest := maliciousArchive(t, func(*profileArchiveDocument) {}, kind)
			if _, err := RestoreProfileArchive(t.Context(), bytes.NewReader(raw), filepath.Join(t.TempDir(), "stage"), identity, digest); err == nil {
				t.Fatal("unsafe tar entry accepted")
			}
		})
	}
}

func TestProfileArchiveRequiresExistingIdentity(t *testing.T) {
	directory := t.TempDir()
	_, identity, _ := transferFixture(t)
	if _, err := WriteProfileArchive(t.Context(), directory, io.Discard, identity); err == nil {
		t.Fatal("empty profile accepted")
	}
	if _, err := os.Stat(filepath.Join(directory, "Runtime")); !os.IsNotExist(err) {
		t.Fatal("manufactured profile identity")
	}
}
