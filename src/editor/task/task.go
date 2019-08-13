package task

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// struct to hold one task (one effect for one color)
type task struct {
	file_in  string
	file_out string
	effect   string
	color    string
	getImg   *bool
	lrs      *layers
	counter  *int
	ctx      *context
}

// each task stores layers for each color for later flattening
type layers struct {
	img  **image.RGBA
	rimg *image.RGBA
	gimg *image.RGBA
	bimg *image.RGBA
	aimg *image.RGBA
}

// locks for our layers and image
type context struct {
	mux  *sync.Mutex
	rmux *sync.Mutex
	gmux *sync.Mutex
	bmux *sync.Mutex
	amux *sync.Mutex
}

// task for output queue
type task_out struct {
	file_out string
	data     image.Image
}

// private methods
// create a new task from parameters
func newTask(in string, out string, fx string, clr string, gI *bool, imgs *layers, ops *int, ctx *context) task {

	return task{in, out, fx, clr, gI, imgs, ops, ctx}
}

// takes a line and creates tasks based on img, effect, and the four colors, then pushes to pipeline
func newTasks(pipeline chan<- task, line string, path string) {

	row := strings.Split(line, ",")
	colors := []string{"R", "G", "B", "A"}
	var mux, rmux, gmux, bmux, amux sync.Mutex
	ops := 0 // keeps track of total operations for the overall image
	var tempTasks []task
	var img, rimg, gimg, bimg, aimg *image.RGBA
	var getImg bool
	getImg = true
	ctx := context{&mux, &rmux, &gmux, &bmux, &amux}
	lrs := layers{&img, rimg, gimg, bimg, aimg}

	for i := 0; i < len(row); i++ {
		row[i] = strings.TrimSpace(row[i])
		// first two columns are filenames, so we add the path
		if i < 2 {
			row[i] = filepath.Join(path, row[i])
		}
		// column two and above are effects to process
		if i > 1 {
			// for each effect, process each color
			for _, clr := range colors {
				ops++
				tempTasks = append(tempTasks, newTask(row[0], row[1], row[i], clr, &getImg, &lrs, &ops, &ctx))
			}
		}
	}
	// feed the array of tasks into the pipeline
	for _, t := range tempTasks {
		pipeline <- t
	}
}

// gets the image filename and converts to RGBA data for manipulation
func copyImg(filename string) *image.RGBA {
	// Open and Decode snippets taken from https://www.devdungeon.com/content/working-images-go
	// with minor changes
	existingImageFile, err := os.Open(filename)
	if err != nil {
		fmt.Printf("file %s cannot be found: %s\n", filename, err)
		return nil
	}
	defer existingImageFile.Close()

	data, err := png.Decode(existingImageFile)
	if err != nil {
		fmt.Printf("cannot decode %s\n", filename)
		return nil
	}
	// end snippet

	// create new blank image and get bounds from original
	b := data.Bounds()
	new_image := image.NewRGBA(b)
	minX := b.Min.X
	minY := b.Min.Y
	maxX := b.Max.X
	maxY := b.Max.Y

	// set color data for each pixel for new image
	for i := minX; i < maxX; i++ {
		for j := minY; j < maxY; j++ {
			new_image.Set(i, j, data.At(i, j))
		}
	}

	return new_image
}

// create blank color layers of the same bounds as original image
func initLayers(t *task) {
	b := (*t.lrs.img).Bounds()
	// we write to each layer independently
	t.ctx.rmux.Lock()
	t.lrs.rimg = image.NewRGBA(b)
	t.ctx.rmux.Unlock()
	t.ctx.gmux.Lock()
	t.lrs.gimg = image.NewRGBA(b)
	t.ctx.gmux.Unlock()
	t.ctx.bmux.Lock()
	t.lrs.bimg = image.NewRGBA(b)
	t.ctx.bmux.Unlock()
	t.ctx.amux.Lock()
	t.lrs.aimg = image.NewRGBA(b)
	t.ctx.amux.Unlock()
}

// takes task and combines color data from each layer and returns a new image with all the colors
func flatten(t *task) image.Image {
	data := t.lrs.img
	b := (*data).Bounds()
	new_image := image.NewRGBA(b)
	minX := b.Min.X
	minY := b.Min.Y
	maxX := b.Max.X
	maxY := b.Max.Y

	for i := minX; i < maxX; i++ {
		for j := minY; j < maxY; j++ {
			// only need one color from each color layer, the rest is empty
			r, _, _, _ := t.lrs.rimg.At(i, j).RGBA()
			_, g, _, _ := t.lrs.gimg.At(i, j).RGBA()
			_, _, b, _ := t.lrs.bimg.At(i, j).RGBA()
			_, _, _, a := t.lrs.aimg.At(i, j).RGBA()
			new_image.Set(i, j, color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)})
		}
	}

	return new_image
}

// performs effect on the 3 by 3 matrix passed to it, returns uint8 of accumulated center pixel
func change(effect string, vs []uint8) uint8 {
	// kernel for sharpening
	S := [][]int{{0, -1, 0}, {-1, 5, -1}, {0, -1, 0}}
	// kernel for edge detection
	E := [][]int{{-1, -1, -1}, {-1, 8, -1}, {-1, -1, -1}}
	temp := 0
	k := 0 // to keep track of place in values matrix vs

	var r uint8
	if effect == "B" {
		// for blurring, we only need to divide by 9. using int to prevent overflow
		t := ((int(vs[0]) + int(vs[1]) + int(vs[2]) + int(vs[3]) + int(vs[4]) + int(vs[5]) + int(vs[6]) + int(vs[7]) + int(vs[8])) / 9)
		// if greater than 255, set to 255 to prevent overflow
		if t > 255 {
			r = 255
		} else {
			r = uint8(t)
		}
	} else if effect == "E" {
		// multiply kernal matrix by values matrix (no need to flip and reverse because of symmetry)
		for i := 0; i < len(E); i++ {
			for j := 0; j < len(E[i]); j++ {
				temp += int(vs[k]) * E[i][j]
				k++
			}
		}
		// prevent illegal value and overflow for uint8
		if temp < 0 {
			temp = 0
		} else if temp > 255 {
			temp = 255
		}
		r = uint8(temp)
	} else if effect == "S" {
		// multiply kernal matrix by values matrix (no need to flip and reverse because of symmetry)
		for i := 0; i < len(S); i++ {
			for j := 0; j < len(S[i]); j++ {
				temp += int(vs[k]) * S[i][j]
				k++
			}
		}
		// prevent illegal value and overflow for uint8
		if temp < 0 {
			temp = 0
		} else if temp > 255 {
			temp = 255
		}
		r = uint8(temp)
	}
	return r
}

// takes pointer to image data, color, and pixel data. returns color pixel data if in bounds
func conform(data **image.RGBA, clr string, x, y, minX, maxX, minY, maxY int) uint8 {
	var p uint32
	var p8 uint8
	// only take the color we need
	switch clr {
	case "R":
		p, _, _, _ = (*data).At(x, y).RGBA()
		p8 = uint8(p)
	case "G":
		_, p, _, _ = (*data).At(x, y).RGBA()
		p8 = uint8(p)
	case "B":
		_, _, p, _ = (*data).At(x, y).RGBA()
		p8 = uint8(p)
	case "A":
		_, _, _, p = (*data).At(x, y).RGBA()
	}
	// 0 if out of bounds
	if x < minX || x > maxX || y < minY || y > maxY {
		p8 = 0
	}
	return p8
}

// bulk of our image work, takes task, effect, and clr, grabs pixel clr data, performs change, updates task images
func convolute(t *task, effect string, clr string) {
	data := t.lrs.img
	b := (*data).Bounds()
	minX := b.Min.X
	minY := b.Min.Y
	maxX := b.Max.X
	maxY := b.Max.Y

	// for each pixel in image bounds
	for i := minX; i < maxX; i++ {
		for j := minY; j < maxY; j++ {
			// if effect is grayscale, only need to set rgb to average
			if effect == "G" {
				r := conform(data, "R", i, j, minX, maxX, minY, maxY)
				g := conform(data, "G", i, j, minX, maxX, minY, maxY)
				b := conform(data, "B", i, j, minX, maxX, minY, maxY)
				avg := (int(r) + int(g) + int(b)) / 3
				// handle each color separately
				switch clr {
				case "R":
					t.ctx.rmux.Lock()
					t.lrs.rimg.Set(i, j, color.RGBA{uint8(avg), 0, 0, 0})
					t.ctx.rmux.Unlock()
				case "G":
					t.ctx.gmux.Lock()
					t.lrs.gimg.Set(i, j, color.RGBA{0, uint8(avg), 0, 0})
					t.ctx.gmux.Unlock()
				case "B":
					t.ctx.bmux.Lock()
					t.lrs.bimg.Set(i, j, color.RGBA{0, 0, uint8(avg), 0})
					t.ctx.bmux.Unlock()
				case "A":
					t.ctx.amux.Lock()
					t.lrs.aimg.Set(i, j, color.RGBA{0, 0, 0, 255})
					t.ctx.amux.Unlock()
				}
				// otherwise, effect is blur, edge detection or sharpen
			} else {
				var vs []uint8
				// get values of surrounding 9 pixels (incl. center) and store in array vs
				for k := -1; k < 2; k++ {
					for l := -1; l < 2; l++ {
						vs = append(vs, conform(data, clr, i+l, j+k, minX, maxX, minY, maxY))
					}
				}
				// handle each color separately
				switch clr {
				case "R":
					r := change(effect, vs)
					t.ctx.rmux.Lock()
					t.lrs.rimg.Set(i, j, color.RGBA{r, 0, 0, 0})
					t.ctx.rmux.Unlock()
				case "G":
					g := change(effect, vs)
					t.ctx.gmux.Lock()
					t.lrs.gimg.Set(i, j, color.RGBA{0, g, 0, 0})
					t.ctx.gmux.Unlock()
				case "B":
					b := change(effect, vs)
					t.ctx.bmux.Lock()
					t.lrs.bimg.Set(i, j, color.RGBA{0, 0, b, 0})
					t.ctx.bmux.Unlock()
				case "A":
					t.ctx.amux.Lock()
					t.lrs.aimg.Set(i, j, color.RGBA{0, 0, 0, 255})
					t.ctx.amux.Unlock()
				}
			}
		}
	}
}

// takes output tasks and writes to a new image file, uses waitgroup to prevent truncation
func writeToFile(output <-chan task_out, wg *sync.WaitGroup) {
	defer wg.Done()

	out_task := <-output

	// Open and Decode snippets taken from https://www.devdungeon.com/content/working-images-go
	// with minor changes
	outputFile, err := os.Create(out_task.file_out)
	if err != nil {
		// Handle error
		fmt.Printf("write failed: %s\n", err)
	}

	err = png.Encode(outputFile, out_task.data)
	if err != nil {
		// Handle error
		fmt.Printf("encoding to file failed: %s\n", err)
	}

	outputFile.Close()
	// end snippet
}

// public methods
// public method for making a task channel
func MakeChan() chan task {
	return make(chan task)
}

// public method for making an output task channel
func MakeOutput() chan task_out {
	return make(chan task_out, 2)
}

// takes a task channel and fills it with new tasks taken from a properly formatted csv file
func OpenChan(filename string, pipeline chan<- task) {
	dat, err := os.Open(filename)
	path := filepath.Dir(filename)

	if err == nil {
		f := bufio.NewScanner(dat)
		defer close(pipeline)
		// takes each csv line, processes, and pushes into the pipeline
		for f.Scan() {
			line := f.Text()
			newTasks(pipeline, line, path)
		}

	} else {
		fmt.Println("file did not open")
	}

}

// takes a task channel, processes each task and pushes to output channel, send Done when done
func Process(tasks <-chan task, output chan task_out, wg *sync.WaitGroup) {
	defer wg.Done()
	var wg2 sync.WaitGroup
	// each task is one effect for one color of an image
	for t := range tasks {
		// copy image and initialize layers if get image is set to true
		t.ctx.mux.Lock()
		if *t.getImg {
			*t.lrs.img = copyImg(t.file_in)
			initLayers(&t)
			*t.getImg = false
		}
		t.ctx.mux.Unlock()
		// process convolution for tasks
		convolute(&t, t.effect, t.color)
		// decrememt operations counter
		t.ctx.mux.Lock()
		*t.counter = *t.counter - 1
		ops_left := *t.counter
		t.ctx.mux.Unlock()
		// if 0 operations left for image, then push to output channel
		if ops_left == 0 {
			out_task := task_out{t.file_out, flatten(&t)}
			output <- out_task
			wg2.Add(1)
			writeToFile(output, &wg2)
		}
	}
	// wait for write waitgroup to finish before finishing process waitgroup
	wg2.Wait()
}

// converts task to string
func (t task) String() string {
	return fmt.Sprintf("%s %s %s %s", t.file_in, t.file_out, t.effect, t.color)
}
