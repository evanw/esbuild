package helpers

// From: http://boost.sourceforge.net/doc/html/boost/hash_combine.html
func HashCombine(seed uint32, hash uint32) uint32 {
	return seed ^ (hash + 0x9e3779b9 + (seed << 6) + (seed >> 2))
}

func HashCombineString(seed uint32, text string) uint32 {
	seed = HashCombine(seed, uint32(len(text)))
	for _, c := range text {
		seed = HashCombine(seed, uint32(c))
	}
	return seed
}
