package main

import (
	"bufio"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// A lab ALI database: the legacy data link a PSAP asks for a 911 caller's record when the call brings
// only its callback number. The request is the ten digits framed by STX and ETX; the answer is the
// record as fixed lines framed the same way, or NO RECORD FOUND (for numbers ending 0000).
//
// The record's layout, for the PSAP's ALI layout settings (line, column, length; 0 = to the line end):
//
//	line 1: callback 1/14, class 17/4
//	line 2: name 1/0
//	line 3: street 1/0
//	line 4: city 1/18, state 20/2
//	line 5: lat 5/11, lon 21/12
//	line 6: unc 5/4, esn 14/4, company 19/0

var aliDigits = regexp.MustCompile(`\d{7,11}`)

// aliRecord is the lab record for a number.
func aliRecord(number string) string {
	if strings.HasSuffix(number, "0000") {
		return "NO RECORD FOUND\r\n" + number
	}
	d := number
	if len(d) == 11 && d[0] == '1' {
		d = d[1:]
	}
	cb := d
	if len(d) == 10 {
		cb = fmt.Sprintf("(%s) %s-%s", d[:3], d[3:6], d[6:])
	}
	lines := []string{
		fmt.Sprintf("%-16s%-4s %s", cb, "RESD", time.Now().Format("01/02 15:04")),
		"LAB SUBSCRIBER " + d[len(d)-4:],
		"1910 WILLIAMSON RD NE",
		fmt.Sprintf("%-19s%s", "ROANOKE", "VA"),
		fmt.Sprintf("LAT %-11s LON %-12s", "+37.29450", "-079.93850"),
		fmt.Sprintf("UNC %-4s ESN %-4s LAB TELCO", "15M", "101"),
	}
	return strings.Join(lines, "\r\n")
}

// runALIDB answers ALI queries on addr until it fails.
func runALIDB(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	fmt.Println("alidb: answering ALI queries on", ln.Addr())
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(10 * time.Second))
			r := bufio.NewReader(c)
			if _, err := r.ReadString(0x02); err != nil {
				return
			}
			req, err := r.ReadString(0x03)
			if err != nil {
				return
			}
			number := aliDigits.FindString(req)
			if number == "" {
				fmt.Printf("alidb: %s asked without a number: %q\n", c.RemoteAddr(), req)
				return
			}
			rec := aliRecord(number)
			fmt.Printf("alidb: %s asked for %s: %s\n", c.RemoteAddr(), number, strings.SplitN(rec, "\r\n", 2)[0])
			c.Write([]byte("\x02" + rec + "\x03"))
		}()
	}
}
