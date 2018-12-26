package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	appdir "github.com/ProtonMail/go-appdir"
	"github.com/cjbassi/gotop/colorschemes"
	"github.com/cjbassi/gotop/src/logging"
	"github.com/cjbassi/gotop/src/widgets"
	w "github.com/cjbassi/gotop/src/widgets"
	ui "github.com/cjbassi/termui"
	docopt "github.com/docopt/docopt.go"
)

var version = "1.7.1"

var (
	colorscheme  = colorschemes.Default
	minimal      = false
	interval     = time.Second
	zoom         = 7
	zoomInterval = 3
	helpVisible  = false
	averageLoad  = false
	percpuLoad   = false
	widgetCount  = 6
	fahrenheit   = false
	bar          = false
	configDir    = appdir.New("gotop").UserConfig()
	logPath      = filepath.Join(configDir, "errors.log")
	stderrLogger = log.New(os.Stderr, "", 0)

	cpu  *w.CPU
	mem  *w.Mem
	proc *w.Proc
	net  *w.Net
	disk *w.Disk
	temp *w.Temp
	help *w.HelpMenu
)

func cliArguments() error {
	usage := `
Usage: gotop [options]

Options:
  -c, --color=NAME      Set a colorscheme.
  -h, --help            Show this screen.
  -m, --minimal         Only show CPU, Mem and Process widgets.
  -r, --rate=RATE       Number of times per second to update CPU and Mem widgets [default: 1].
  -v, --version         Show version.
  -p, --percpu          Show each CPU in the CPU widget.
  -a, --averagecpu      Show average CPU in the CPU widget.
  -f, --fahrenheit      Show temperatures in fahrenheit.
  -b, --bar             Show a statusbar with the time.

Colorschemes:
  default
  default-dark (for white background)
  solarized
  monokai
`

	args, err := docopt.ParseArgs(usage, os.Args[1:], version)
	if err != nil {
		return err
	}

	if val, _ := args["--color"]; val != nil {
		if err := handleColorscheme(val.(string)); err != nil {
			return err
		}
	}
	averageLoad, _ = args["--averagecpu"].(bool)
	percpuLoad, _ = args["--percpu"].(bool)

	minimal, _ = args["--minimal"].(bool)
	if minimal {
		widgetCount = 3
	}

	bar, _ = args["--bar"].(bool)

	rateStr, _ := args["--rate"].(string)
	rate, err := strconv.ParseFloat(rateStr, 64)
	if err != nil {
		return fmt.Errorf("invalid rate parameter")
	}
	if rate < 1 {
		interval = time.Second * time.Duration(1/rate)
	} else {
		interval = time.Second / time.Duration(rate)
	}
	fahrenheit, _ = args["--fahrenheit"].(bool)

	return nil
}

func handleColorscheme(cs string) error {
	switch cs {
	case "default":
		colorscheme = colorschemes.Default
	case "solarized":
		colorscheme = colorschemes.Solarized
	case "monokai":
		colorscheme = colorschemes.Monokai
	case "default-dark":
		colorscheme = colorschemes.DefaultDark
	default:
		if colorscheme, err := getCustomColorscheme(cs); err != nil {
			colorscheme = colorscheme
			return err
		}
	}
	return nil
}

// getCustomColorscheme	tries to read a custom json colorscheme from {configDir}/{name}.json
func getCustomColorscheme(name string) (colorschemes.Colorscheme, error) {
	var colorscheme colorschemes.Colorscheme
	filePath := filepath.Join(configDir, name+".json")
	dat, err := ioutil.ReadFile(filePath)
	if err != nil {
		return colorscheme, fmt.Errorf("colorscheme file not found")
	}
	err = json.Unmarshal(dat, &colorscheme)
	if err != nil {
		return colorscheme, fmt.Errorf("could not parse colorscheme file")
	}
	return colorscheme, nil
}

func setupGrid() {
	width := 12000
	height := 12000

	ui.Body.Cols = width
	ui.Body.Rows = height

	if minimal {
		ui.Body.Set(0, 0, width, height/2, cpu)

		if bar {
			ui.Body.Set(0, height/2, width/2, height-1, mem)
			ui.Body.Set(width/2, height/2, width, height-1, proc)

			bar := widgets.NewStatusbar()
			ui.Body.Set(0, height, width, height+2, bar)
		} else {
			ui.Body.Set(0, height/2, width/2, height, mem)
			ui.Body.Set(width/2, height/2, width, height, proc)
		}
	} else {
		ui.Body.Set(0, 0, width, height/3, cpu)

		ui.Body.Set(0, height/3, width/3, height/2, disk)
		ui.Body.Set(0, height/2, width/3, (width*2)/3, temp)
		ui.Body.Set(width/3, height/3, width, (height*2)/3, mem)

		if bar {
			ui.Body.Set(0, (height*2)/3, width/2, height-1, net)
			ui.Body.Set(width/2, (height*2)/3, width, height-1, proc)

			bar := widgets.NewStatusbar()
			ui.Body.Set(0, height, width, height+2, bar)
		} else {
			ui.Body.Set(0, (height*2)/3, width/2, height, net)
			ui.Body.Set(width/2, (height*2)/3, width, height, proc)
		}
	}
}

func termuiColors() {
	ui.Theme.Fg = ui.Color(colorscheme.Fg)
	ui.Theme.Bg = ui.Color(colorscheme.Bg)
	ui.Theme.LabelFg = ui.Color(colorscheme.BorderLabel)
	ui.Theme.LabelBg = ui.Color(colorscheme.Bg)
	ui.Theme.BorderFg = ui.Color(colorscheme.BorderLine)
	ui.Theme.BorderBg = ui.Color(colorscheme.Bg)

	ui.Theme.TableCursor = ui.Color(colorscheme.ProcCursor)
	ui.Theme.Sparkline = ui.Color(colorscheme.Sparkline)
	ui.Theme.GaugeColor = ui.Color(colorscheme.DiskBar)
}

func widgetColors() {
	mem.LineColor["Main"] = ui.Color(colorscheme.MainMem)
	mem.LineColor["Swap"] = ui.Color(colorscheme.SwapMem)

	var keys []string
	for key := range cpu.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	i := 0
	for _, v := range keys {
		if i >= len(colorscheme.CPULines) {
			// assuming colorscheme for CPU lines is not empty
			i = 0
		}
		c := colorscheme.CPULines[i]
		cpu.LineColor[v] = ui.Color(c)
		i++
	}

	if !minimal {
		temp.TempLow = ui.Color(colorscheme.TempLow)
		temp.TempHigh = ui.Color(colorscheme.TempHigh)
	}
}

func initWidgets() {
	var wg sync.WaitGroup
	wg.Add(widgetCount)

	go func() {
		cpu = w.NewCPU(interval, zoom, averageLoad, percpuLoad)
		wg.Done()
	}()
	go func() {
		mem = w.NewMem(interval, zoom)
		wg.Done()
	}()
	go func() {
		proc = w.NewProc()
		wg.Done()
	}()
	if !minimal {
		go func() {
			net = w.NewNet()
			wg.Done()
		}()
		go func() {
			disk = w.NewDisk()
			wg.Done()
		}()
		go func() {
			temp = w.NewTemp(fahrenheit)
			wg.Done()
		}()
	}

	wg.Wait()
}

func eventLoop() {
	drawTicker := time.NewTicker(interval).C

	// handles kill signal sent to gotop
	sigTerm := make(chan os.Signal, 2)
	signal.Notify(sigTerm, os.Interrupt, syscall.SIGTERM)

	uiEvents := ui.PollEvents()

	previousKey := ""

	for {
		select {
		case <-sigTerm:
			return
		case <-drawTicker:
			if !helpVisible {
				ui.Render(ui.Body)
			}
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "?":
				helpVisible = !helpVisible
				if helpVisible {
					ui.Clear()
					ui.Render(help)
				} else {
					ui.Render(ui.Body)
				}
			case "h":
				if !helpVisible {
					zoom += zoomInterval
					cpu.Zoom = zoom
					mem.Zoom = zoom
					ui.Render(cpu, mem)
				}
			case "l":
				if !helpVisible {
					if zoom > zoomInterval {
						zoom -= zoomInterval
						cpu.Zoom = zoom
						mem.Zoom = zoom
						ui.Render(cpu, mem)
					}
				}
			case "<Escape>":
				if helpVisible {
					helpVisible = false
					ui.Render(ui.Body)
				}
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				ui.Body.Width, ui.Body.Height = payload.Width, payload.Height
				ui.Body.Resize()
				ui.Clear()
				if helpVisible {
					ui.Render(help)
				} else {
					ui.Render(ui.Body)
				}

			case "<MouseLeft>":
				payload := e.Payload.(ui.Mouse)
				proc.Click(payload.X, payload.Y)
				ui.Render(proc)
			case "<MouseWheelUp>", "<Up>", "k":
				proc.Up()
				ui.Render(proc)
			case "<MouseWheelDown>", "<Down>", "j":
				proc.Down()
				ui.Render(proc)
			case "g", "<Home>":
				if previousKey == "g" {
					proc.Top()
					ui.Render(proc)
				}
			case "G", "<End>":
				proc.Bottom()
				ui.Render(proc)
			case "<C-d>":
				proc.HalfPageDown()
				ui.Render(proc)
			case "<C-u>":
				proc.HalfPageUp()
				ui.Render(proc)
			case "<C-f>":
				proc.PageDown()
				ui.Render(proc)
			case "<C-b>":
				proc.PageUp()
				ui.Render(proc)
			case "d":
				if previousKey == "d" {
					proc.Kill()
				}
			case "<Tab>":
				proc.Tab()
				ui.Render(proc)
			case "m", "c", "p":
				proc.ChangeSort(e)
				ui.Render(proc)
			}

			if previousKey == e.ID {
				previousKey = ""
			} else {
				previousKey = e.ID
			}
		}
	}
}

func setupLogging() (*os.File, error) {
	// make the config directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to make the configuration directory: %v", err)
	}
	// open the log file
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0660)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	// log time, filename, and line number
	log.SetFlags(log.Ltime | log.Lshortfile)
	// log to file
	log.SetOutput(lf)

	return lf, nil
}

func main() {
	lf, err := setupLogging()
	if err != nil {
		stderrLogger.Fatalf("failed to setup logging: %v", err)
	}
	defer lf.Close()

	if err := cliArguments(); err != nil {
		stderrLogger.Fatalf("failed to parse cli args: %v", err)
	}

	termuiColors() // need to do this before initializing widgets so that they can inherit the colors
	initWidgets()
	widgetColors()
	help = w.NewHelpMenu()

	if err := ui.Init(); err != nil {
		stderrLogger.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	logging.StderrToLogfile(lf)

	setupGrid()
	ui.Render(ui.Body)

	eventLoop()
}
