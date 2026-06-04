package utils

func ShortHash(hash string, n int) string {
	if len(hash) <= n {
		return hash
	}
	return hash[:n]
}
