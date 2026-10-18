package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
)

func TestPasswordRoundTrip(t *testing.T) {
	ctx := t.Context()
	password := "预约管理🔑\x00" + strings.Repeat("长密码", 30)
	first, err := HashPassword(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(ctx, password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("hashes reused the same salt")
	}
	if !strings.HasPrefix(first, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatal("unexpected password profile")
	}
	for _, hash := range []string{first, second} {
		if ok, err := VerifyPassword(ctx, hash, password); err != nil || !ok {
			t.Fatalf("correct password: match=%v err=%v", ok, err)
		}
		if ok, err := VerifyPassword(ctx, hash, password+"x"); err != nil || ok {
			t.Fatalf("wrong password: match=%v err=%v", ok, err)
		}
	}
}

func TestDummyPasswordProfile(t *testing.T) {
	if ok, err := VerifyPassword(t.Context(), dummyHash, "dummy password for timing"); err != nil || !ok {
		t.Fatalf("dummy fixture no longer matches the active profile: match=%v err=%v", ok, err)
	}
	if err := VerifyDummy(t.Context(), "incorrect password"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidPasswordHashes(t *testing.T) {
	// A cancelled context distinguishes validation errors from any attempt to
	// acquire the hashing semaphore. Bad stored data must never control cost.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for name, hash := range map[string]string{
		"empty":            "",
		"legacy profile":   strings.Replace(dummyHash, "m=19456,t=2,p=1", "m=65536,t=3,p=2", 1),
		"wrong version":    strings.Replace(dummyHash, "v=19", "v=16", 1),
		"wrong variant":    strings.Replace(dummyHash, "argon2id", "argon2i", 1),
		"zero lanes":       strings.Replace(dummyHash, "p=1", "p=0", 1),
		"excessive memory": strings.Replace(dummyHash, "m=19456", "m=4294967295", 1),
		"missing payload":  passwordHashPrefix,
		"bad salt":         strings.Replace(dummyHash, "cmVnaXN0ZXItbWFuYWdlcg", "!mVnaXN0ZXItbWFuYWdlcg", 1),
		"short salt":       strings.Replace(dummyHash, "cmVnaXN0ZXItbWFuYWdlcg", "YWJj", 1),
		"short key":        dummyHash[:len(dummyHash)-1],
		"bad key":          dummyHash[:len(dummyHash)-1] + "!",
		"extra field":      dummyHash + "$extra",
	} {
		t.Run(name, func(t *testing.T) {
			if ok, err := VerifyPassword(ctx, hash, "password"); ok || !errors.Is(err, errInvalidPasswordHash) {
				t.Fatalf("invalid hash: match=%v err=%v", ok, err)
			}
		})
	}
}

func TestPasswordConcurrencyAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		if !passwordSlots.TryAcquire(2) {
			t.Fatal("password slots were not released by a previous operation")
		}
		held := true
		defer func() {
			if held {
				passwordSlots.Release(2)
			}
		}()
		if passwordSlots.TryAcquire(1) {
			passwordSlots.Release(1)
			t.Fatal("more than two password operations are allowed")
		}
		operations := []func(context.Context) error{
			func(ctx context.Context) error { _, err := HashPassword(ctx, "new password"); return err },
			func(ctx context.Context) error {
				_, err := VerifyPassword(ctx, dummyHash, "wrong password")
				return err
			},
			func(ctx context.Context) error { return VerifyDummy(ctx, "unknown account") },
		}
		done := make(chan error, len(operations))
		for _, operation := range operations {
			go func() { done <- operation(ctx) }()
		}
		synctest.Wait()
		if len(done) != 0 {
			t.Fatal("a password operation bypassed the shared concurrency limit")
		}
		cancel()
		synctest.Wait()
		for range operations {
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("waiting operation did not cancel: %v", err)
			}
		}
		passwordSlots.Release(2)
		held = false
		for _, operation := range operations {
			if err := operation(t.Context()); err != nil {
				t.Fatal(err)
			}
			if !passwordSlots.TryAcquire(2) {
				t.Fatal("completed operation leaked a password slot")
			}
			passwordSlots.Release(2)
		}
	})
}

func BenchmarkPasswordHash(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := HashPassword(b.Context(), "benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPasswordVerify(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := VerifyPassword(b.Context(), dummyHash, "benchmark password"); err != nil {
			b.Fatal(err)
		}
	}
}
