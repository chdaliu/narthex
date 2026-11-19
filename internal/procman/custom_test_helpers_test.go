package procman

import "narthex/internal/store"

// startForTest keeps the old positional start shape for the existing tests.
// hosts is accepted for source compatibility but no longer used by any
// launcher.
func startForTest(m *Manager, kind, dir, id string, hosts []string, noAuth bool) (int, int, error) {
	return m.Start(StartSpec{
		Card:   store.Card{ID: id, Kind: kind},
		Dir:    dir,
		NoAuth: noAuth,
	})
}
