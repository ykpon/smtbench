package kernels

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Mix maps kernel name to work units per tick.
type Mix map[string]int

func ParseMix(s string) (Mix, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("пустая смесь нагрузки")
	}
	m := Mix{}
	for _, part := range strings.Split(s, ",") {
		name, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("ожидается имя=число, получено %q", part)
		}
		if !slices.Contains(order, name) {
			return nil, fmt.Errorf("неизвестная нагрузка %q, доступны: %s", name, strings.Join(order, ", "))
		}
		if _, dup := m[name]; dup {
			return nil, fmt.Errorf("нагрузка %q указана дважды", name)
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > 10000 {
			return nil, fmt.Errorf("%s: число единиц должно быть от 1 до 10000", name)
		}
		m[name] = n
	}
	return m, nil
}

func (m Mix) String() string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(parts, ",")
}

func (m Mix) NeedsMem() bool { return m["mem"] > 0 }
