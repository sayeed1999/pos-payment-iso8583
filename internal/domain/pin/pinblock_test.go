package pin

import "testing"

func TestRoundTrip(t *testing.T) {
	blk, err := Encrypt("1234", "4111111111111111")
	if err != nil || len(blk) != 8 {
		t.Fatalf("%v len %d", err, len(blk))
	}
	got, err := Decrypt(blk, "4111111111111111")
	if err != nil || got != "1234" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestBlockIsBoundToThePAN(t *testing.T) {
	blk, _ := Encrypt("1234", "4111111111111111")
	if got, err := Decrypt(blk, "5555555555554444"); err == nil && got == "1234" {
		t.Fatal("PIN block decrypted correctly under the wrong PAN")
	}
}

func TestRejectsBadPINs(t *testing.T) {
	for _, p := range []string{"123", "12a4", "1234567890123"} {
		if _, err := Encrypt(p, "4111111111111111"); err == nil {
			t.Errorf("PIN %q accepted", p)
		}
	}
}
