//go:build !windows

package device

func Scan() ([]Device, error) {
	return nil, nil
}
