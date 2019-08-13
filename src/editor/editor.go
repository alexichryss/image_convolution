//
// Problem 3: reads in a red-light violation csv file, prints out violation per year per season
//
package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"./task"
)

// helper function to print usage info
func printUsage() {
	fmt.Printf("\nUsage: editor <csv file> [-p[=num of threads]]\n\n")
	fmt.Printf("<csv file>  = csv file to process\n")
	fmt.Printf("-p[=num of threads] = flag to parallelize")
	fmt.Printf(" w/ optional number of threads (inc. main)\n")
	return
}

// takes number of threads, csv filename, and waitgroup and creates appropriate number of workers
func worker(threads int, csv string, wg *sync.WaitGroup) {
	tasks := task.MakeChan()
	output := task.MakeOutput()

	// for every thread minus the main thread, spawn a go routine and wait
	for i := 0; i < threads-1; i++ {
		wg.Add(1)
		go task.Process(tasks, output, wg)
	}
	// for main thread, spawn a routine and wait
	wg.Add(1)
	go task.Process(tasks, output, wg)

	// now process csv file and push to waiting threads
	task.OpenChan(csv, tasks)

}

func main() {
	// get command line arguments
	args := os.Args[1:]
	pattern := "-p($|\\s)|-p=\\d{1,2}($|\\s)"
	var threads int

	// if no argument, print usage
	if len(args) == 0 {
		printUsage()
		return
		// if just filename, threads = 1
	} else if len(args) == 1 {
		threads = 1

		// else, make sure flag is set properly to -p[=n] using regex, and get threads
	} else if reg, _ := regexp.Match(pattern, []byte(args[1])); len(args) == 2 && reg {
		flag := strings.Split(args[1], "=")
		threads = runtime.NumCPU()
		if len(flag) > 1 {
			var err error
			threads, err = strconv.Atoi(flag[1])
			if err != nil {
				threads = runtime.NumCPU()
			}
		}
		// otherwise, number of arguments is incorrect, so print usage
	} else {
		printUsage()
		return
	}

	// create waitgroup, spawn workers according to threads, then wait for workers to finish
	var wg sync.WaitGroup
	worker(threads, args[0], &wg)
	wg.Wait()

}
