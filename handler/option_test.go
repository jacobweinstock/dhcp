package handler

import "testing"

func TestXxx(t *testing.T) {
	if fn, err := Convert(54, "192.168.2.2"); err == nil {
		t.Log(fn)
	} else {
		t.Error(err)
	}

	if fn, err := Convert(1, "255.255.255.0"); err == nil {
		t.Log(fn)
	} else {
		t.Error(err)
	}

	if fn, err := Convert(2, "10"); err == nil {
		t.Log(fn)
	} else {
		t.Error(err)
	}

	if fn, err := Convert(3, "192.168.2.1"); err == nil {
		t.Log(fn)
	} else {
		t.Error(err)
	}

	t.Fail()
}
