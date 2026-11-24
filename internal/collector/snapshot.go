package collector

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"reflect"
	"strings"
)

func snapshot(ctx context.Context, root *os.Root, id string, data json.RawMessage) (changed bool, err error) {
	if err := root.Mkdir(id, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return false, errors.New("snapshot directory unavailable")
	}
	// Refuse aliases between recording folders as well as paths outside raw.
	info, err := root.Lstat(id)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("snapshot directory invalid")
	}
	directory, err := root.OpenRoot(id)
	if err != nil {
		return false, errors.New("snapshot directory unavailable")
	}
	defer directory.Close()
	const name = "recording.json"
	previous, err := regularFile(directory, name, 0, false)
	if err == nil && sameJSON(previous, data) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, errors.New("prior snapshot unreadable")
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return false, errors.New("invalid snapshot JSON")
	}
	formatted.WriteByte('\n')
	if err := atomicReplace(ctx, directory, name, formatted.Bytes()); err != nil {
		return false, err
	}
	return true, nil
}

func atomicReplace(ctx context.Context, directory *os.Root, name string, data []byte) (err error) {
	tempName := ".recording-" + rand.Text() + ".tmp"
	file, err := directory.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("snapshot temporary file unavailable")
	}
	defer func() {
		if cleanupErr := directory.Remove(tempName); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, errors.New("snapshot temporary cleanup failed"))
		}
	}()
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		file.Close()
		return errors.New("snapshot write failed")
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return errors.New("snapshot sync failed")
	}
	if err := file.Close(); err != nil {
		return errors.New("snapshot close failed")
	}
	if ctx.Err() != nil {
		return errInterrupted
	}
	if err := directory.Rename(tempName, name); err != nil {
		return errors.New("snapshot replacement failed")
	}
	return nil
}

func sameJSON(a, b []byte) bool {
	decode := func(data []byte) (any, error) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return nil, errors.New("trailing JSON")
		}
		return normalizeNumbers(value), nil
	}
	left, err := decode(a)
	if err != nil {
		return false
	}
	right, err := decode(b)
	return err == nil && reflect.DeepEqual(left, right)
}

type exactNumber struct {
	coefficient string
	exponent    string
}

// Normalize decimal numbers without float64 rounding or expanding huge
// exponents. Thus 1000, 1e3 and 1000.0 compare equal, including unknown fields.
func normalizeNumbers(value any) any {
	switch value := value.(type) {
	case json.Number:
		mantissa, exponent, _ := strings.Cut(strings.ToLower(string(value)), "e")
		power := new(big.Int)
		if exponent != "" {
			power.SetString(exponent, 10)
		}
		if before, after, found := strings.Cut(mantissa, "."); found {
			mantissa = before + after
			power.Sub(power, big.NewInt(int64(len(after))))
		}
		negative := strings.HasPrefix(mantissa, "-")
		mantissa = strings.TrimLeft(strings.TrimPrefix(mantissa, "-"), "0")
		if mantissa == "" {
			return exactNumber{"0", "0"}
		}
		trimmed := strings.TrimRight(mantissa, "0")
		power.Add(power, big.NewInt(int64(len(mantissa)-len(trimmed))))
		if negative {
			trimmed = "-" + trimmed
		}
		return exactNumber{trimmed, power.String()}
	case []any:
		for i := range value {
			value[i] = normalizeNumbers(value[i])
		}
	case map[string]any:
		for key := range value {
			value[key] = normalizeNumbers(value[key])
		}
	}
	return value
}
