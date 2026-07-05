package hltv

import "testing"

func TestEssentialCookie(t *testing.T) {
	cf := "ABC.test"
	bm := "bm123"
	raw := "cf_clearance=" + cf + "; __cf_bm=" + bm + "; other=junk; session=xyz"
	got := EssentialCookie(raw)
	want := "cf_clearance=" + cf + "; __cf_bm=" + bm
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildCookieFromParts(t *testing.T) {
	cf := "6vox8MiD4pH65HibSAsCh6.TEST"
	bm := "i7wc7wEmLKtok2F_lSE5RuLzKLuXcvr5cDHExUhTXek-1780782807"
	want := "cf_clearance=" + cf + "; __cf_bm=" + bm

	got := BuildCookieFromParts(cf, bm)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	got = BuildCookieFromParts("cf_clearance:"+cf, "__cf_bm:"+bm)
	if got != want {
		t.Fatalf("with prefixes: got %q want %q", got, want)
	}

	pcf, pbm := ParseCookieParts(want)
	if pcf != cf || pbm != bm {
		t.Fatalf("parse: cf=%q bm=%q", pcf, pbm)
	}
}

func TestNormalizeCookie(t *testing.T) {
	val := "6vox8MiD4pH65HibSAsCh6.TEST"
	bm := "i7wc7wEmLKtok2F_lSE5RuLzKLuXcvr5cDHExUhTXek-1780782807"

	tests := []struct {
		in   string
		want string
	}{
		{val, "cf_clearance=" + val},
		{"cf_clearance:" + val, "cf_clearance=" + val},
		{"cf_clearance=" + val, "cf_clearance=" + val},
		{"cf_clearance:" + val + "\n__cf_bm:" + bm, "cf_clearance=" + val + "; __cf_bm=" + bm},
		{"cf_clearance=" + val + "; __cf_bm=" + bm, "cf_clearance=" + val + "; __cf_bm=" + bm},
	}

	for _, tc := range tests {
		got := NormalizeCookie(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeCookie(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
