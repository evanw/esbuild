package helpers

import "sync"

// Each call to "Enter(i)" doesn't start until "Leave(i-1)" is called
type Serializer struct {
	flags []sync.WaitGroup
}

func MakeSerializer(count int) Serializer {
	flags := make([]sync.WaitGroup, count)
	for i := 0; i < count; i++ {
		flags[i].Add(1)
	}
	return Serializer{flags: flags}
}

func (s *Serializer) Enter(i int) {
	if i > 0 {
		s.flags[i-1].Wait()
	}
}

func (s *Serializer) Leave(i int) {
	s.flags[i].Done()
}
