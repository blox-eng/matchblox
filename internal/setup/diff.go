package setup

import "strings"

// diff is a line diff of a and b: "+" for a new line, "-" for a removed one,
// " " for one line of context around each change, and "…" between changes.
func diff(a, b string) string {
	x, y := lines(a), lines(b)
	// The longest common subsequence, from the end.
	n, m := len(x), len(y)
	if n*m > 4_000_000 { // a file this large is shown whole
		return prefix(x, "-") + prefix(y, "+")
	}
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type op struct {
		mark byte
		line string
	}
	var ops []op
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && x[i] == y[j]:
			ops = append(ops, op{' ', x[i]})
			i, j = i+1, j+1
		case i < n && (j == m || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', x[i]})
			i++
		default:
			ops = append(ops, op{'+', y[j]})
			j++
		}
	}
	near := func(k int) bool {
		for d := -1; d <= 1; d++ {
			if k+d >= 0 && k+d < len(ops) && ops[k+d].mark != ' ' {
				return true
			}
		}
		return false
	}
	var out strings.Builder
	gap := false
	for k, o := range ops {
		if o.mark == ' ' && !near(k) {
			gap = true
			continue
		}
		if gap && out.Len() > 0 {
			out.WriteString("…\n")
		}
		gap = false
		out.WriteByte(o.mark)
		out.WriteString(o.line)
		out.WriteByte('\n')
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func prefix(ls []string, mark string) string {
	var out strings.Builder
	for _, l := range ls {
		out.WriteString(mark + l + "\n")
	}
	return out.String()
}
