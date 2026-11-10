package research

type AcceptResult struct{ Written, Backup, Branch string }

func Accept(s Store, name string, force bool) (AcceptResult, error) { return AcceptResult{}, nil }
