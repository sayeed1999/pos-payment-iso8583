package domain

import "testing"

func TestMaskPAN(t *testing.T) {
	if got := MaskPAN("4111111111111111"); got != "411111******1111" {
		t.Fatal(got)
	}
}
