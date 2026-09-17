package components

import "testing"

func TestModalOpenFlag(t *testing.T) {
	cases := []struct {
		props ModalProps
		want  string
	}{
		{ModalProps{AlpineOpen: "isOpen"}, "isOpen"},
		{ModalProps{AlpineClose: "showAddModal = false"}, "showAddModal"},
		{ModalProps{AlpineClose: "!isBulkUploading && (uploadModal = false)"}, "uploadModal"},
		{ModalProps{AlpineClose: "closeEdit()"}, ""},
		{ModalProps{}, ""},
	}
	for _, c := range cases {
		if got := modalOpenFlag(c.props); got != c.want {
			t.Errorf("modalOpenFlag(%+v) = %q, want %q", c.props, got, c.want)
		}
	}
}
