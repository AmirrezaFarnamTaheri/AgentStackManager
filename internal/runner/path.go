package runner

type defaultPathRefresher struct{}

func (defaultPathRefresher) Refresh() error { return refreshProcessPath() }
