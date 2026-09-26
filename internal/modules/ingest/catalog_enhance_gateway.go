package ingest

// SetEnhancer installs the AI matching port. Leaving it unset means the stage
// is skipped, which is the same path a disabled gateway takes.
func (s *Service) SetEnhancer(e Enhancer) { s.enhancer = e }

// SetMatchMemory installs the shared decision cache. It is optional: without it
// the stage still works and simply pays for every question.
func (s *Service) SetMatchMemory(m MatchMemory) { s.memory = m }

// AIAvailable reports whether the import screen may offer the AI switch.
func (s *Service) AIAvailable() bool { return s != nil && s.enhancer != nil }
