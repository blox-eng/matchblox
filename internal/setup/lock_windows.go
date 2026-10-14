package setup

func withKeysLock(home string, fn func() error) error { return fn() }
