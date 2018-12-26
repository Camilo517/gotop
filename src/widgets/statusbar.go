package widgets

import (
	"os"
	"time"

	ui "github.com/cjbassi/termui"
)

type Statusbar struct {
	*ui.Block
}

func NewStatusbar() *Statusbar {
	self := &Statusbar{ui.NewBlock()}
	self.BorderFg = 0
	return self
}

func (self *Statusbar) Buffer() *ui.Buffer {
	buf := self.Block.Buffer()

	hostname, _ := os.Hostname()
	buf.SetString(1, self.Block.Y/2, hostname, ui.Color(7), self.Bg)

	t := time.Now()
	_time := t.Format("15:04:05")
	buf.SetString(1+(self.Block.X-len(_time))/2, self.Block.Y/2, _time, ui.Color(7), self.Bg)

	buf.SetString(self.Block.X-5, self.Block.Y/2, "gotop", ui.Color(7), self.Bg)

	return buf
}
