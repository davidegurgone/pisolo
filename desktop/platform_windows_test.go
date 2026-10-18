//go:build windows

package main

import "testing"

func TestParseLidValues(t *testing.T) {
	// Italian output: labels are localized, only the hex values matter.
	out := `GUID combinazione per il risparmio di energia: 381b4222-f694-41f0-9685-ff5bb260df2e  (Bilanciato)
  GUID sottogruppo: 4f971e89-eebd-4455-a8de-9e59040e7347  (Pulsanti di alimentazione e coperchio)
    GUID impostazione alimentazione: 5ca83367-6e45-459f-a27b-476b1d01c936  (Azione chiusura coperchio)
      Indice impostazione possibile: 000
      Nome descrittivo impostazione possibile: Non fare nulla
      Indice impostazione possibile: 001
      Nome descrittivo impostazione possibile: Sospensione
    Indice impostazione corrente alimentazione CA: 0x00000001
    Indice impostazione corrente alimentazione CC: 0x00000003`
	ac, dc, err := parseLidValues(out)
	if err != nil || ac != 1 || dc != 3 {
		t.Fatalf("got %d %d %v", ac, dc, err)
	}
	if _, _, err := parseLidValues("nothing here"); err != errNoLid {
		t.Fatal("expected errNoLid")
	}
}
