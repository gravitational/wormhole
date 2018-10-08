package main

import "fmt"

var (
	commitHash string
	timestamp  string
	gitTag     string
)

func main() {
	fmt.Println("hello world")
	fmt.Println("commitHash: ", commitHash)
	fmt.Println("timestamp: ", timestamp)
	fmt.Println("gitTag: ", gitTag)
}
