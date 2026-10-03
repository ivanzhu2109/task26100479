package watchstats

// viewer 聚合同一观众全部会话的覆盖。
type viewer struct {
	sessions map[string]*session
	cov      coverage
	total    int64
}

func newViewer() *viewer {
	return &viewer{sessions: make(map[string]*session)}
}
