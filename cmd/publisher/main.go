package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"panestra.local/plugins/sdk/secret"
	"path/filepath"
)

type Metadata struct {
	SchemaVersion   int               `json:"schemaVersion"`
	Product         string            `json:"product"`
	Version         string            `json:"version"`
	ReleaseSequence uint64            `json:"releaseSequence"`
	APIVersion      int               `json:"apiVersion"`
	SHA256          string            `json:"sha256"`
	Size            int64             `json:"size"`
	Files           map[string]string `json:"files"`
	PluginID        string            `json:"pluginId"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func writeNew(file string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(raw)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func run() error {
	keyPath := flag.String("key", "", "DPAPI-protected signing key")
	generate := flag.Bool("generate-key", false, "generate signing identity")
	input := flag.String("input", "", "built plugin directory")
	output := flag.String("output", "", "output package prefix")
	sequence := flag.Uint64("sequence", 0, "monotonic per-plugin sequence")
	flag.Parse()
	if *keyPath == "" {
		return errors.New("--key is required")
	}
	if *generate {
		pub, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		defer clear(key)
		protected, err := secret.Protect(key)
		if err != nil {
			return err
		}
		if err := writeNew(*keyPath, protected); err != nil {
			return err
		}
		return writeNew(*keyPath+".pub", []byte(hex.EncodeToString(pub)))
	}
	if *input == "" || *output == "" || *sequence == 0 {
		return errors.New("--input, --output and positive --sequence are required")
	}
	protected, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	plain, err := secret.Unprotect(protected)
	if err != nil {
		return err
	}
	defer clear(plain)
	if len(plain) != ed25519.PrivateKeySize {
		return errors.New("invalid signing key")
	}
	key := ed25519.PrivateKey(plain)
	manifestBytes, err := os.ReadFile(filepath.Join(*input, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Engine  struct {
			Entry string `json:"entry"`
		} `json:"engine"`
	}
	if json.Unmarshal(manifestBytes, &manifest) != nil || manifest.ID == "" || manifest.Version == "" || filepath.Base(manifest.Engine.Entry) != manifest.Engine.Entry {
		return errors.New("invalid built plugin manifest")
	}
	for _, suffix := range []string{".zip", ".json", ".sig"} {
		if _, err := os.Stat(*output + suffix); !os.IsNotExist(err) {
			return errors.New("immutable output exists")
		}
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0700); err != nil {
		return err
	}
	packageFile, err := os.OpenFile(*output+".zip", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(packageFile)
	files := map[string]string{}
	for _, name := range []string{"manifest.json", manifest.Engine.Entry} {
		raw, err := os.ReadFile(filepath.Join(*input, name))
		if err != nil {
			archive.Close()
			packageFile.Close()
			return err
		}
		if len(raw) > 128*1024*1024 {
			return errors.New("plugin file too large")
		}
		out, err := archive.Create(name)
		if err != nil {
			return err
		}
		if _, err := out.Write(raw); err != nil {
			return err
		}
		hash := sha256.Sum256(raw)
		files[name] = hex.EncodeToString(hash[:])
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := packageFile.Close(); err != nil {
		return err
	}
	f, err := os.Open(*output + ".zip")
	if err != nil {
		return err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	f.Close()
	if err != nil {
		return err
	}
	meta := Metadata{1, "Panestra", manifest.Version, *sequence, 1, hex.EncodeToString(hash.Sum(nil)), size, files, manifest.ID}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := writeNew(*output+".json", raw); err != nil {
		return err
	}
	if err := writeNew(*output+".sig", []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw)))); err != nil {
		return err
	}
	fmt.Println("Signed plugin:", manifest.ID, manifest.Version)
	return nil
}
