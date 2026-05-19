package main

import (
	"flag"
	"fmt"
	"reflect"
)

func main() {
	t := reflect.ValueOf(flag.Usage).Type()
	fmt.Printf("flag.Usage type: %s\n", t)
	fmt.Printf("  NumIn: %d\n", t.NumIn())
	fmt.Printf("  NumOut: %d\n", t.NumOut())
	fmt.Printf("  IsVariadic: %v\n", t.IsVariadic())
}
