// Simple CLI interface to Electronic Safe v2
//    https://bdsm.spuddy.org/writings/Safe_v2/
// designed for Emlalock to set a random password
// and then embed it into an image.
// This can now be the unlock image.
//
// Commands:
//  ./picture_lock {common} -lock locked_image.jpg
//  ./picture_lock {common} -test locked_image.jpg
//  ./picture_lock {common} -unlock locked_image.jpg
//  ./picture_lock {common} -status
//
// Common options:
//  [-user username -pass password] -safe safe.name
//
// These can also be set in $HOME/.picture_lock (or %HOMEDIR%%HOMEPATH%
// on windows as a JSON file so they don't need to be passed each time
//
// e.g.
// {
// 	"Safe": "safe.local",
// 	"User": "username",
// 	"Pass": "password"
// }
//
// A safe name is mandatory, username/password are optional but if the
// safe requires them then you need to specify them

package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/oned"
	"github.com/tkanos/gonfig"
)

// What characters we allow for safe passwords.  In theory anything except
// a : should work, but we're gonna be more restrictive
const pswdstring = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// Information we read from the config file
type Configuration struct {
	Safe string
	User string
	Pass string
}

var configuration Configuration

// Make these global so they're easy to use, rather than passing them through
// a chain of main->{function}->talk_to_safe
var username, passwd, safe string

//////////////////////////////////////////////////////////////////////
//
// Utility functions
//
//////////////////////////////////////////////////////////////////////

func abort(str string) {
	fmt.Fprintln(os.Stderr, "\n"+str)
	os.Exit(-1)
}

// Where do config files live?
func UserHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home + "\\"
	}
	return os.Getenv("HOME") + "/"
}

func rot13(r rune) rune {
	if r >= 'a' && r <= 'z' {
		// Rotate lowercase letters 13 places.
		if r > 'm' {
			return r - 13
		} else {
			return r + 13
		}
	} else if r >= 'A' && r <= 'Z' {
		// Rotate uppercase letters 13 places.
		if r > 'M' {
			return r - 13
		} else {
			return r + 13
		}
	}
	// Do nothing.
	return r
}

func addLabel(img *image.RGBA, x, y int, label string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{255, 0, 0, 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{fixed.I(x), fixed.I(y)},
	}
	d.DrawString(label)
}

//////////////////////////////////////////////////////////////////////
//
// Talk to Safe
//
//////////////////////////////////////////////////////////////////////

func talk_to_safe(cmd string) string {
	url := "http://" + safe + "/safe/?" + cmd
	// fmt.Println("We want to to " + url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		// Ensure error doesn't have cmd in it...
		msg := strings.Replace(err.Error(), cmd, "*******", 1)
		abort("Got error setting up http request: " + msg)
	}

	req.SetBasicAuth(username, passwd)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		msg := strings.Replace(err.Error(), cmd, "*******", 1)
		abort("Problems talking to the safe: " + msg)
	}

	// Get the response as a string
	//   http://dlintw.github.io/gobyexample/public/http-client.html
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		abort("Problems getting response from safe: " + err.Error())
	}
	res := string(body)

	if resp.StatusCode != 200 {
		abort("Bad result from safe: " + resp.Status + "\n" + res)
	}
	return res
}

//////////////////////////////////////////////////////////////////////
//
// Main functions
//
//////////////////////////////////////////////////////////////////////

func lock(dest string) {
	//	fmt.Println("Creating a new lock")

	// Generate a random password
	b := make([]byte, 30)
	for i := range b {
		b[i] = pswdstring[rand.Intn(len(pswdstring))]
	}
	new_pswd := string(b)

	// DEBUG
	// new_pswd = "hello"
	// fmt.Println("DEBUG: new_pswd is `hello`")

	enc := oned.NewCode128Writer()
	barcode, err := enc.Encode("LOCKPSW:"+strings.Map(rot13, new_pswd), gozxing.BarcodeFormat_CODE_128, 800, 140, nil)

	if err != nil {
		abort("Unable to generate barcode: " + err.Error())
	}

	// Create a new image of double the height
	bounds := barcode.Bounds()
	bounds.Max.Y = 2 * bounds.Max.Y
	dst := image.NewRGBA(bounds)
	// Set image to all white
	draw.Draw(dst, bounds, &image.Uniform{color.RGBA{255,255,255,255}}, image.ZP, draw.Src)
	// Stick the barcode at the top
	draw.Draw(dst, barcode.Bounds(), barcode, image.Point{}, draw.Src)

	// Now draw some text
	addLabel(dst, 5, bounds.Max.Y-70, "This barcode represents a combination that has been loaded into")
	addLabel(dst, 5, bounds.Max.Y-50, "a safe similar to the one at https://bdsm.spuddy.org/writings/Safe_v3/")
	addLabel(dst, 5, bounds.Max.Y-30, "using software from https://github.com/bdsm-spuddy/chaster-picture-safe/")
	addLabel(dst, 5, bounds.Max.Y-10, "This is a pretty strong solution, stronger than a realtor lock!")

	// Save the new image
	f, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		abort("We could not create the image file: " + err.Error())
	}

	err = jpeg.Encode(f, dst, nil)
	if err != nil {
		abort("We could not write the image file: " + err.Error())
	}
	f.Close()

	// Lock the safe
	res := talk_to_safe("lock=1&lock1=" + new_pswd + "&lock2=" + new_pswd)
	if res != "Safe locked" {
		abort("Problem locking safe: " + res)
	}

	// Check the password was accepted
	res = talk_to_safe("pwtest=1&unlock=" + new_pswd)
	if res != "Passwords match" {
		abort("Unable to verify lock worked: " + res)
	}

	fmt.Println(dest + " created.")
}

func unlock(file string, tst bool) {
	lock_file, err := os.Open(file)
	if err != nil {
		abort(err.Error())
	}

	img, _, _ := image.Decode(lock_file)
	bmp, _ := gozxing.NewBinaryBitmapFromImage(img)

	barcode := oned.NewCode128Reader()
	result, _ := barcode.Decode(bmp, nil)
	psw := result.String()

	if strings.HasPrefix(psw, "LOCKPSW:") {
		psw = strings.Map(rot13, psw[8:])
	} else {
		abort("This is not a valid password image")
	}

	cmd := "unlock_all"
	if tst {
		cmd = "pwtest"
	}

	// fmt.Println("Decoded password is: " + psw)
	fmt.Println(talk_to_safe(cmd + "=1&unlock=" + psw))
}

func main() {
	// Let's seed our random function
	rand.Seed(time.Now().UnixNano())

	// Try and find the config file
	config_file := UserHomeDir() + ".picture_lock"
	if _, err := os.Stat(config_file); err == nil {
		// fmt.Println("Using configuration file " + config_file)

		parse := gonfig.GetConf(config_file, &configuration)
		if parse != nil {
			abort("Error parsing " + config_file + ": " + parse.Error())
		}
	}

	flag.StringVar(&username, "user", "", "Username to talk to safe (optional)")
	flag.StringVar(&passwd, "pass", "", "Password to talk to safe (optional)")
	flag.StringVar(&safe, "safe", "", "Safe Address")

	lockflag := flag.Bool("lock", false, "Lock the safe, create new image")
	unlockflag := flag.Bool("unlock", false, "Unlock the safe with image")
	testflag := flag.Bool("test", false, "Test the image can unlock the safe")
	statusflag := flag.Bool("status", false, "Request current safe status")

	flag.Parse()

	// If the user didn't define these three things, use values
	// from the config file
	if username == "" {
		username = configuration.User
	}

	if passwd == "" {
		passwd = configuration.Pass
	}

	if safe == "" {
		safe = configuration.Safe
	}

	// Safe better be defined!
	if safe == "" {
		abort("No safe name passed")
	}

	if *statusflag {
		fmt.Println(talk_to_safe("status=1"))
		os.Exit(0)
	}

	args := flag.Args()

	if len(args) == 0 {
		abort("Missing filename; use the -h option for help")
	} else if len(args) != 1 {
		abort("Only one filename is allowed and must be the last value;\n  use the \"-h\" option for help")
	}

	filename := args[0]

	if *lockflag {
		lock(filename)
	} else if *unlockflag {
		unlock(filename, false)
	} else if *testflag {
		unlock(filename, true)
	} else {
		abort("Command should be -lock or -unlock or -test; use -h for help")
	}
}
