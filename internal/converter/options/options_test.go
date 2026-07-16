package options

import "testing"

func TestFilterRequiredFieldExtension(t *testing.T) {
	t.Run("sets the extension number", func(t *testing.T) {
		opts, err := FromString("filter-required-field-extension=50002")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.FilterRequiredFieldExtension != 50002 {
			t.Fatalf("got %d, want 50002", opts.FilterRequiredFieldExtension)
		}
	})

	t.Run("defaults to zero (disabled)", func(t *testing.T) {
		opts, err := FromString("trim-unused-types")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.FilterRequiredFieldExtension != 0 {
			t.Fatalf("got %d, want 0", opts.FilterRequiredFieldExtension)
		}
	})

	for _, raw := range []string{"", "0", "-1", "abc"} {
		t.Run("rejects "+raw, func(t *testing.T) {
			if _, err := FromString("filter-required-field-extension=" + raw); err == nil {
				t.Fatalf("expected error for %q, got nil", raw)
			}
		})
	}
}
