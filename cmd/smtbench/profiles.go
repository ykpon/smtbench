package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"smtbench/internal/kernels"
)

func cmdProfiles([]string) int {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Профиль\tСмесь (единиц на тик)\tПамять\tОписание")
	for _, p := range kernels.Profiles() {
		mem := "—"
		if p.Mix.NeedsMem() {
			mem = fmt.Sprintf("%d МБ", p.MemSizeMB)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Mix, mem, p.Description)
	}
	w.Flush()
	fmt.Println("\nСвоя смесь: --mix int=50,mem=40,fp=10. Одна единица ≈ 1 мкс на эталонном Xeon E5-2667 v2.")
	return 0
}
