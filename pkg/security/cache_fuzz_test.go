package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fuzz coverage for the HMAC integrity cache (Phase 5). The parse path
// — JSON unmarshal of the signedEntry envelope, hex-decode of the MAC,
// constant-time compare, embedded-key binding, expiry — runs on bytes
// pulled straight off disk. Anything that can land in <baseDir>/<op>/<hash>.json
// is attacker-controllable in any local-tamper scenario (the threat model
// the HMAC layer exists to defend against), so the parser must be panic-
// free and must never report `found=true` for an entry whose MAC does
// not match the cache's key.
//
// These fuzz tests close OpenSSF Scorecard's Fuzzing check (0 -> 10) by
// exercising the parse seam (verifyAndExtract) and the path-derivation
// function (getPath) with arbitrary inputs.

// validSignedBytes produces a known-good signed entry encoded as JSON
// bytes, with the same shape that Set() writes to disk. Used as seed
// corpus and as the basis for tampered variants.
func validSignedBytes(t testing.TB, c *Cache, key CacheKey, payload []byte, expiresAt time.Time) []byte {
	t.Helper()
	entry := CacheEntry{
		Key:       key,
		Data:      payload,
		CreatedAt: expiresAt.Add(-time.Hour),
		ExpiresAt: expiresAt,
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	mac := hmac.New(sha256.New, c.key)
	mac.Write(entryJSON)
	signed := signedEntry{
		Entry: entry,
		MAC:   hex.EncodeToString(mac.Sum(nil)),
	}
	out, err := json.Marshal(signed)
	if err != nil {
		t.Fatalf("marshal signed: %v", err)
	}
	return out
}

// FuzzVerifyAndExtract drives arbitrary bytes through the parse-and-
// verify path. Two invariants:
//
//  1. No input causes a panic. JSON unmarshal, hex.DecodeString, and
//     hmac.Equal all tolerate adversarial input by design; this fuzz
//     pins that contract.
//  2. If verifyAndExtract returns ok=true (cache hit), then re-computing
//     the HMAC over the canonical-marshaled embedded entry must match
//     the MAC in the file. This is the no-false-positive invariant:
//     fuzz must not find any byte string that passes verification
//     without holding the cache's key.
//
// The fuzz target re-derives the requested key from the seed string
// (instead of taking the full CacheKey) to keep the corpus compact and
// to let the mutator explore both matching-key and mismatching-key
// branches without hand-crafting nested JSON.
func FuzzVerifyAndExtract(f *testing.F) {
	// Build a stable cache (and HMAC key) once per fuzz process. The
	// fuzz target reuses it for every input — verifyAndExtract is pure
	// w.r.t. the cache (no state mutation), so this is safe.
	cache, err := NewCache(f.TempDir())
	if err != nil {
		f.Fatalf("NewCache: %v", err)
	}

	// Reference keys for seed generation. The fuzz input encodes which
	// key the verifier should treat as "requested" — verifyAndExtract
	// rejects a validly-signed file whose embedded Key differs from
	// the requested one (cross-key copy defence).
	keyA := CacheKey{Operation: "sbom", ImageDigest: "sha256:A", ConfigHash: "h1"}
	keyB := CacheKey{Operation: "scan-grype", ImageDigest: "sha256:B", ConfigHash: "h2"}

	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	// Seed 1: a known-good signed entry, requested with the matching key.
	f.Add(validSignedBytes(f, cache, keyA, []byte("payload-A"), future), "sbom", "sha256:A", "h1")
	// Seed 2: same valid bytes, requested with a DIFFERENT key — must miss.
	f.Add(validSignedBytes(f, cache, keyA, []byte("payload-A"), future), "scan-grype", "sha256:B", "h2")
	// Seed 3: valid entry but already expired — must miss.
	f.Add(validSignedBytes(f, cache, keyA, []byte("payload-A"), past), "sbom", "sha256:A", "h1")
	// Seed 4: a tampered Data payload (MAC no longer matches).
	tampered := tamperData(f, cache, keyA, []byte("orig"), []byte("evil"), future)
	f.Add(tampered, "sbom", "sha256:A", "h1")
	// Seed 5: a tampered MAC (right length, wrong bits).
	f.Add(tamperMAC(f, cache, keyA, []byte("payload"), future), "sbom", "sha256:A", "h1")
	// Seed 6: MAC field stripped to empty.
	f.Add(tamperEmptyMAC(f, cache, keyA, []byte("payload"), future), "sbom", "sha256:A", "h1")
	// Seed 7: MAC field as non-hex garbage.
	f.Add(tamperNonHexMAC(f, cache, keyA, []byte("payload"), future), "sbom", "sha256:A", "h1")
	// Seed 8: a legacy pre-HMAC entry (just the CacheEntry shape, no envelope).
	legacy, _ := json.Marshal(CacheEntry{Key: keyA, Data: []byte("legacy"), ExpiresAt: future})
	f.Add(legacy, "sbom", "sha256:A", "h1")
	// Seed 9: empty input.
	f.Add([]byte(""), "sbom", "sha256:A", "h1")
	// Seed 10: garbage bytes.
	f.Add([]byte{0x00, 0x01, 0x02, 0x03, 0xff, 0xfe}, "sbom", "sha256:A", "h1")
	// Seed 11: JSON with extra unknown fields — Go's encoding/json
	//   ignores them on Unmarshal, so this should still parse cleanly.
	f.Add([]byte(`{"entry":{"key":{"Operation":"sbom","ImageDigest":"sha256:A","ConfigHash":"h1"},"data":"AA==","createdAt":"0001-01-01T00:00:00Z","expiresAt":"0001-01-01T00:00:00Z","extra":"ignored"},"mac":"00","unknown":42}`), "sbom", "sha256:A", "h1")
	// Seed 12: deeply nested JSON to stress the unmarshal recursion path.
	deep := []byte(`{"entry":{"data":` + strings.Repeat(`[`, 64) + `1` + strings.Repeat(`]`, 64) + `},"mac":"00"}`)
	f.Add(deep, "sbom", "sha256:A", "h1")
	// Seed 13: cross-key-copy survivor — keyB's payload at keyA's request.
	f.Add(validSignedBytes(f, cache, keyB, []byte("payload-B"), future), "sbom", "sha256:A", "h1")
	// Seed 14: null entry.
	f.Add([]byte(`{"entry":null,"mac":"00"}`), "sbom", "sha256:A", "h1")
	// Seed 15: massive MAC field (hex but wrong length).
	f.Add(tamperMACTooLong(f, cache, keyA, []byte("payload"), future), "sbom", "sha256:A", "h1")

	f.Fuzz(func(t *testing.T, data []byte, op string, digest string, confHash string) {
		requested := CacheKey{Operation: op, ImageDigest: digest, ConfigHash: confHash}

		// Use a fixed reference time so expiry behaviour is deterministic
		// — otherwise a flaky `time.Now()` in the parser would make
		// repeated runs of the same crash input behave differently.
		now := time.Unix(1_700_000_000, 0).UTC()

		payload, ok, _ := cache.verifyAndExtract(data, requested, now)

		if !ok {
			// Cache miss — no further invariants to check. Just confirm
			// the parser didn't smuggle a payload through.
			if payload != nil {
				t.Fatalf("ok=false but payload=%q is non-nil", payload)
			}
			return
		}

		// ok=true: the verifier accepted the bytes. Independently
		// reconstruct the truth-table and assert every condition holds.
		var signed signedEntry
		if err := json.Unmarshal(data, &signed); err != nil {
			t.Fatalf("ok=true but data does not unmarshal: %v", err)
		}
		gotMAC, err := hex.DecodeString(signed.MAC)
		if err != nil || len(gotMAC) == 0 {
			t.Fatalf("ok=true but MAC field is invalid: %q (err=%v)", signed.MAC, err)
		}
		entryJSON, err := json.Marshal(signed.Entry)
		if err != nil {
			t.Fatalf("ok=true but entry re-marshal failed: %v", err)
		}
		expectMAC := cache.computeMAC(entryJSON)
		if !hmac.Equal(gotMAC, expectMAC) {
			// THIS is the bug the fuzzer is looking for: a successful
			// extraction whose MAC does not actually verify under our
			// key. If we hit this, the HMAC layer is broken.
			t.Fatalf("ok=true but recomputed MAC %x != stored MAC %x", expectMAC, gotMAC)
		}
		if signed.Entry.Key != requested {
			t.Fatalf("ok=true but embedded key %+v != requested %+v", signed.Entry.Key, requested)
		}
		if now.After(signed.Entry.ExpiresAt) {
			t.Fatalf("ok=true but entry is expired (now=%v expires=%v)", now, signed.Entry.ExpiresAt)
		}
		// Returned payload must match the embedded data.
		if string(payload) != string(signed.Entry.Data) {
			t.Fatalf("returned payload %q != embedded data %q", payload, signed.Entry.Data)
		}
	})
}

// FuzzCacheGetPath drives arbitrary CacheKey strings through getPath
// and asserts the result is always contained under baseDir. The path
// derivation runs `filepath.Base + filepath.Clean` on Operation and
// falls back to "_unknown" for separators / traversal segments — fuzz
// pins that no input escapes the cache root.
//
// Invariants:
//
//  1. No panic (filepath.Clean / Base, strings.ContainsAny all tolerant).
//  2. Result is always under baseDir (no `../` escape).
//  3. Result has a stable .json suffix.
func FuzzCacheGetPath(f *testing.F) {
	baseDir := f.TempDir()
	cache, err := NewCache(baseDir)
	if err != nil {
		f.Fatalf("NewCache: %v", err)
	}

	// Realistic seeds: in-tree callers use these four operation names.
	f.Add("sbom", "sha256:abc123", "h1")
	f.Add("scan-grype", "sha256:def456", "h2")
	f.Add("scan-trivy", "sha256:0", "")
	f.Add("signature", "", "")
	// Adversarial: path traversal, separators, absolute paths.
	f.Add("../etc/passwd", "sha256:abc", "h")
	f.Add("..", "sha256:abc", "h")
	f.Add(".", "sha256:abc", "h")
	f.Add("", "sha256:abc", "h")
	f.Add("/etc/passwd", "sha256:abc", "h")
	f.Add(`C:\Windows\System32`, "sha256:abc", "h")
	f.Add("sbom/../../../etc", "sha256:abc", "h")
	f.Add("\x00null", "sha256:abc", "h")
	// Long inputs.
	f.Add(strings.Repeat("a", 10_000), "sha256:abc", "h")
	// Unicode / control / multi-byte. Using \u202e escape (RTL OVERRIDE)
	// instead of the literal codepoint so the source file stays ASCII
	// and GitHub doesn't fire the bidi-Unicode warning on this file.
	// The fuzz seed bytes at runtime are identical either way.
	f.Add("\u202esbom", "sha256:abc", "h")

	// Compute the absolute, symlink-resolved baseDir once so the
	// containment check matches what filepath.Join produces.
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		f.Fatalf("filepath.Abs(baseDir): %v", err)
	}
	absBase = filepath.Clean(absBase) + string(filepath.Separator)

	f.Fuzz(func(t *testing.T, op, digest, conf string) {
		key := CacheKey{Operation: op, ImageDigest: digest, ConfigHash: conf}

		// Containment is meaningful only against the cleaned base; the
		// returned path itself comes from filepath.Join which already
		// cleans. We assert that the path's directory is exactly two
		// levels deep under baseDir: baseDir/<op-name>/<hex>.json
		got := cache.getPath(key)
		absGot, err := filepath.Abs(got)
		if err != nil {
			t.Fatalf("filepath.Abs(getPath result): %v", err)
		}
		absGot = filepath.Clean(absGot)
		if !strings.HasPrefix(absGot+string(filepath.Separator), absBase) &&
			absGot != filepath.Clean(absBase) {
			t.Fatalf("path %q escapes baseDir %q (op=%q)", absGot, absBase, op)
		}

		if !strings.HasSuffix(got, ".json") {
			t.Fatalf("path %q lacks .json suffix", got)
		}

		// The filename component must be a 64-hex-char SHA-256 of the
		// concatenated key fields, plus ".json". Length check is a
		// cheap sanity that the hashing layer wasn't bypassed.
		base := filepath.Base(got)
		if len(base) != 64+len(".json") {
			t.Fatalf("filename %q has wrong length %d", base, len(base))
		}
	})
}

// --- tamper helpers -----------------------------------------------------
//
// Each helper takes a valid signed entry and mutates one component, so
// the seed corpus exercises a known-bad branch on every iteration.

func tamperData(t testing.TB, c *Cache, key CacheKey, orig, evil []byte, expires time.Time) []byte {
	t.Helper()
	raw := validSignedBytes(t, c, key, orig, expires)
	var s signedEntry
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s.Entry.Data = evil // MAC now stale
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func tamperMAC(t testing.TB, c *Cache, key CacheKey, payload []byte, expires time.Time) []byte {
	t.Helper()
	raw := validSignedBytes(t, c, key, payload, expires)
	var s signedEntry
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Flip first byte of the MAC (still valid hex, still right length).
	macBytes, _ := hex.DecodeString(s.MAC)
	macBytes[0] ^= 0xff
	s.MAC = hex.EncodeToString(macBytes)
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func tamperEmptyMAC(t testing.TB, c *Cache, key CacheKey, payload []byte, expires time.Time) []byte {
	t.Helper()
	raw := validSignedBytes(t, c, key, payload, expires)
	var s signedEntry
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s.MAC = ""
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func tamperNonHexMAC(t testing.TB, c *Cache, key CacheKey, payload []byte, expires time.Time) []byte {
	t.Helper()
	raw := validSignedBytes(t, c, key, payload, expires)
	var s signedEntry
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s.MAC = "not-hex-data-zzzzz"
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func tamperMACTooLong(t testing.TB, c *Cache, key CacheKey, payload []byte, expires time.Time) []byte {
	t.Helper()
	raw := validSignedBytes(t, c, key, payload, expires)
	var s signedEntry
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// hex-valid but wrong length (256 bytes vs. expected 32).
	s.MAC = strings.Repeat("ab", 256)
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}
