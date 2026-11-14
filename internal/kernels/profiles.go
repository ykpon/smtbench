package kernels

type Profile struct {
	Name        string
	Description string
	Mix         Mix
	MemSizeMB   int
}

var profiles = []Profile{
	{"int", "синтетический: ветвистый целочисленный код (логика сущностей)", Mix{"int": 100}, 0},
	{"fp", "синтетический: скалярная арифметика с плавающей точкой (физика)", Mix{"fp": 100}, 0},
	{"avx", "синтетический: 256-битные AVX-вычисления", Mix{"avx": 100}, 0},
	{"mem", "синтетический: случайный обход памяти (кэш и задержка RAM)", Mix{"mem": 100}, 64},
	{"source", "синтетический, приближение Source/CS2: логика, физика, немного памяти", Mix{"int": 50, "fp": 35, "mem": 15}, 16},
	{"minecraft", "синтетический, приближение Minecraft (Java): логика и много обходов памяти", Mix{"int": 45, "fp": 10, "mem": 45}, 64},
	{"rust", "синтетический, приближение Rust (Unity): логика, физика и память поровну", Mix{"int": 35, "fp": 35, "mem": 30}, 64},
}

func Profiles() []Profile { return profiles }

func ProfileByName(name string) (Profile, bool) {
	for _, p := range profiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}
