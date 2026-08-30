package Accounts

// Hosted profile directories used to be keyed by the hosted-account UUID, so
// deleting and recreating a portal user (or a hosted account) orphaned the
// collected corpus — feature stats, history tables, storm coverage — even
// though the game account itself was unchanged. Profiles are therefore keyed
// by the game identity: canonical world + game player ID, which is stable no
// matter how often the portal side is recreated.
//
// A brand-new runtime cannot know its player ID before the first login, so it
// stages under Accounts/<runtime-id>. Once the session binds its identity the
// supervisor performs a one-time rebind: it stops the runtime, folds the
// staging directory into Players/<world>-p<playerID> (adopting any existing
// corpus for that player), records the binding, and restarts the runtime on
// the player directory. Every later runtime for the same player — including
// one created after a portal-side delete/recreate — resolves straight to the
// corpus through the persisted binding or through a fresh rebind.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	playerDirsName          = "Players"
	playerBindingsFileName  = "player-bindings.json"
	playerBindingsSchemaVer = 1
)

type playerBindingsFile struct {
	SchemaVersion int               `json:"schemaVersion"`
	Bindings      map[string]string `json:"bindings"`
}

// playerKey renders the stable identity as a filesystem-safe directory name,
// e.g. "ep-live-us1-game-goodgamestudios-com-p145046".
func playerKey(worldID string, playerID int64) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(worldID)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	key := strings.Trim(builder.String(), "-")
	for strings.Contains(key, "--") {
		key = strings.ReplaceAll(key, "--", "-")
	}
	return fmt.Sprintf("%s-p%d", key, playerID)
}

func loadPlayerBindings(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read player bindings: %w", err)
	}
	var file playerBindingsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("decode player bindings: %w", err)
	}
	if file.Bindings == nil {
		file.Bindings = map[string]string{}
	}
	return file.Bindings, nil
}

func savePlayerBindings(path string, bindings map[string]string) error {
	encoded, err := json.MarshalIndent(playerBindingsFile{
		SchemaVersion: playerBindingsSchemaVer, Bindings: bindings,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode player bindings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create bindings directory: %w", err)
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, encoded, 0o600); err != nil {
		return fmt.Errorf("write player bindings: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("publish player bindings: %w", err)
	}
	return nil
}

// mergeStagingIntoPlayerDir folds a short-lived staging profile into an
// adopted player directory: history rows append after the corpus (staging is
// always younger), and the staging session files — which carry the freshest
// installed credentials — win. Everything else defers to the corpus. The
// staging directory is renamed aside afterwards so nothing is destroyed.
func mergeStagingIntoPlayerDir(staging, player string, stamp string) error {
	historyDir := filepath.Join(staging, "History")
	entries, err := os.ReadDir(historyDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read staging history: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		if err := appendFile(
			filepath.Join(historyDir, entry.Name()),
			filepath.Join(player, "History", entry.Name()),
		); err != nil {
			return fmt.Errorf("merge history %s: %w", entry.Name(), err)
		}
	}
	sessionDir := filepath.Join(staging, "Session")
	sessionEntries, err := os.ReadDir(sessionDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read staging session: %w", err)
	}
	for _, entry := range sessionEntries {
		if entry.IsDir() {
			continue
		}
		if err := copyFile(
			filepath.Join(sessionDir, entry.Name()),
			filepath.Join(player, "Session", entry.Name()),
		); err != nil {
			return fmt.Errorf("carry session file %s: %w", entry.Name(), err)
		}
	}
	if err := os.Rename(staging, staging+".superseded-"+stamp); err != nil {
		return fmt.Errorf("retire staging directory: %w", err)
	}
	return nil
}

func appendFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
