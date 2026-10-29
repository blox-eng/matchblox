package app

// homeLogin is the login of the console's home host.
func homeLogin(m Model) hostLogin { return m.loginOf(m.opt.Host) }
