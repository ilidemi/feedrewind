package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/tools/go/packages"
)

func main() {
	err := os.Chdir("../..")
	if err != nil {
		panic(err)
	}

	cfg := packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedModule,
	}
	pkgs, err := packages.Load(&cfg, ".")
	if err != nil {
		panic(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		panic(errors.New("Packages contain errors"))
	}
	if len(pkgs) != 1 {
		panic(errors.New("Expected exactly one package"))
	}

	packagesSeen := make(map[string]bool)
	var queue []*packages.Package
	queue = append(queue, pkgs[0])
	packagesSeen[pkgs[0].PkgPath] = true
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]

		for _, goFile := range pkg.GoFiles {
			fmt.Println(goFile)
		}

		for _, importPkg := range pkg.Imports {
			if !packagesSeen[importPkg.PkgPath] {
				queue = append(queue, importPkg)
				packagesSeen[importPkg.PkgPath] = true
			}
		}
	}
}
