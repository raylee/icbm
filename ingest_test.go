package main

import "testing"

func TestClamp(t *testing.T) {
	if clamp(-1.0, 0.0, 1.0) < -0.1 {
		t.Error("Clamp is not enforcing the bottom end")
	}

	if clamp(2, 0, 1) > 1.1 {
		t.Error("Clamp is not enforcing the bottom end")
	}

	// keeping it easy as 1/2^n is perfectly representable in standard floating point
	if clamp(0.5, 0, 1) != 0.5 {
		t.Error("Clamp is not allowing safe values through")
	}
}
