package svc

import (
	"flag"
	"fmt"
	"os"
)

// Cmd represents a sub command, allowing to define subcommand
// flags and runnable to run once arguments match the subcommand
// requirements.
type Cmd interface {
	Flags(*flag.FlagSet) *flag.FlagSet
	Run(args []string) error
}

type Commands struct {
	// the name of program
	program string

	// the flags of global
	flags *flag.FlagSet

	// A map of all of the registered sub-commands.
	list []*cmdInstance

	// Matching subcommand.
	matchingCmd *cmdInstance

	// Arguments to call subcommand's runnable.
	args []string

	// Flag to determine whether help is
	// asked for subcommand or not
	flagHelp bool
}

func New(program string, flags *flag.FlagSet) *Commands {
	return &Commands{program: program, flags: flags}
}

type cmdInstance struct {
	name        string
	description string
	command     Cmd
}

// Registers a Cmd for the provided sub-command name. E.g. name is the
// `status` in `git status`.
func (c *Commands) On(name, description string, command Cmd) {
	c.list = append(c.list, &cmdInstance{
		name:        name,
		description: description,
		command:     command,
	})
}

// Prints the usage.
func (c *Commands) Usage() {
	if len(c.list) == 0 {
		// no subcommands
		fmt.Fprintf(os.Stderr, "使用方法: %s [选项]\n", c.program)
		c.flags.PrintDefaults()
		return
	}

	fmt.Fprintf(os.Stderr, "使用方法: %s [选项] 子命令 [选项] \n\n", c.program)
	fmt.Fprintf(os.Stderr, "子命令列表:\n")
	for _, subcmd := range c.list {
		fmt.Fprintf(os.Stderr, "  %-15s %s\n", subcmd.name, subcmd.description)
	}

	// Returns the total number of globally registered flags.
	count := 0
	c.flags.VisitAll(func(flag *flag.Flag) {
		count++
	})

	if count > 0 {
		fmt.Fprintf(os.Stderr, "\n选项:\n")
		c.flags.PrintDefaults()
	}
	fmt.Fprintf(os.Stderr, "\n查看子命令的帮助: %s 子命令 -h\n", c.program)
}

func (c *Commands) SubcommandUsage(subcmd *cmdInstance) {
	fmt.Fprintf(os.Stderr, "%s\r\n", subcmd.description)
	// should only output sub command flags, ignore h flag.
	fs := subcmd.command.Flags(flag.NewFlagSet(subcmd.name, flag.ContinueOnError))
	flagCount := 0
	fs.VisitAll(func(flag *flag.Flag) { flagCount++ })
	if flagCount > 0 {
		fmt.Fprintf(os.Stderr, "使用方法: %s %s [选项]\n", c.program, subcmd.name)
		fs.PrintDefaults()
	}
}

// Parses the flags and leftover arguments to match them with a
// sub-command. Evaluate all of the global flags and register
// sub-command handlers before calling it. Sub-command handler's
// `Run` will be called if there is a match.
// A usage with flag defaults will be printed if provided arguments
// don't match the configuration.
// Global flags are accessible once Parse executes.
func (c *Commands) Parse(args []string) {
	// if there are no subcommands registered,
	// return immediately
	if len(c.list) < 1 {
		return
	}

	if len(args) < 1 {
		c.Usage()
		os.Exit(1)
	}

	name := args[0]
	var subcmd *cmdInstance
	for _, sub := range c.list {
		if sub.name == name {
			subcmd = sub
			break
		}
	}

	if subcmd == nil {
		c.Usage()
		os.Exit(1)
	}

	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs = subcmd.command.Flags(fs)
	fs.BoolVar(&c.flagHelp, "h", false, "")
	fs.BoolVar(&c.flagHelp, "?", false, "")
	fs.BoolVar(&c.flagHelp, "help", false, "")

	c.matchingCmd = subcmd
	fs.Usage = func() {
		c.SubcommandUsage(subcmd)
	}
	fs.Parse(args[1:])
	c.args = fs.Args()
}

// Runs the subcommand's runnable. If there is no subcommand
// registered, it silently returns.
func (c *Commands) Run() {
	if c.matchingCmd != nil {
		if c.flagHelp {
			c.SubcommandUsage(c.matchingCmd)
			return
		}

		if err := c.matchingCmd.command.Run(c.args); err != nil {
			var code = -1
			var help = false
			if e, ok := err.(*Error); ok {
				code = e.Code
				help = e.Help
			}

			fmt.Fprintf(os.Stderr, "FATAL: %s", err.Error())
			if help {
				c.SubcommandUsage(c.matchingCmd)
			}
			os.Exit(code)
			return
		}
	}
}

// Parses flags and run's matching subcommand's runnable.
func (c *Commands) ParseAndRun(args []string) {
	c.Parse(args)
	c.Run()
}

var Default = New(os.Args[0], flag.CommandLine)

func On(name, description string, command Cmd) {
	Default.On(name, description, command)
}

func Usage() {
	Default.Usage()
}

func Parse() {
	flag.Usage = Default.Usage
	flag.Parse()
	Default.Parse(flag.Args())
}

func Run() {
	Default.Run()
}

func ParseAndRun() {
	Parse()
	Run()
}

type Error struct {
	Code    int
	Message string
	Help    bool
}

func (e *Error) Error() string {
	return e.Message
}
