//
// Written By : Daniel Antonsen (@root4loot)
//
// Distributed Under MIT License
// Copyrights (C) 2019 root4loot
//

package scope

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/gookit/color"
)

// Match contains lists of regex submatches
type Match struct {
	Includes [][]string
	Excludes [][]string
	Counter  int
}

// Parse function takes a slice containing scope file data and
// applies regex to each line in order to extract targets from scope-
// matched targets are split into groups varying on type (host, url, iprange, etc)
// Returns a Match object
func Parse(m Match, scopes, source []string, silent bool, incTag, exTag string, bbaas bool) Match {
	var exclude bool

	// Set Tag used to indicate beginning of Includes
	if len(incTag) == 0 {
		incTag = "!INCLUDE"
	}

	// Set Tag used to indicate beginning of Excludes
	if len(exTag) == 0 {
		exTag = "!EXCLUDE"
	}

	r1 := regexp.MustCompile(`([a-z3]+:\/\/)?(\*\.)?(\*?[a-z0-9-.]+(\.[a-z]+))(:\d+)?([A-Za-z0-9-._~:/?#@!$&'*+=]+)?`)
	// Groups: 1.  [ftp]://sub.example.com:25/d/foo.bar    // scheme
	//         2.   ftp://[*.]example.com:25/d/foo.bar     // wildcarded subdomain
	//	       3.   ftp://[sub.example.com]:25/d/foo.bar   // host
	//         4.   ftp://sub.example[.com]:25/d/foo.bar   // extension
	//         5.   ftp://sub.example.com[:25]/d/foo.bar   // port
	//         6.   ftp://sub.example.com:25[/d/foo.bar]   // path

	r1 := regexp.MustCompile(`([a-z3]+:\/\/)?([a-z]+\.)?(\*\.)?(\*?[a-z0-9-.]+(\.[a-z]+))(:\d+)?([A-Za-z0-9-._~:/?#@!$&'*+=]+)?`)
	// Groups: 1.  [ftp]://sub.example.com:25/d/foo.bar    		// scheme
	//         2.   ftp://[sub].example.com:25/d/foo.bar        // first subdomain
	//         3.   ftp://[*.]example.com:25/d/foo.bar     		// wildcarded subdomain
	//	       4.   ftp://sub.[sub.example.com]:25/d/foo.bar   	// second, third.. subdomain + toplevel
	//         5.   ftp://sub.example[.com]:25/d/foo.bar   		// extension
	//         6.   ftp://sub.example.com[:25]/d/foo.bar   		// port
	//         7.   ftp://sub.example.com:25[/d/foo.bar]   		// path

	r2 := regexp.MustCompile(`((\d+\.\d+\.\d+\.)(\d+)-(\d+))`)
	// Matches IP-Range
	// Groups: 1.  [d.d.d.d-d]
	//         2.  [d.d.d].d-d
	//         3.   d.d.d.[d]-d
	//         4.   d.d.d.d-[d]

	r3 := regexp.MustCompile(`([0-9]+[\.0-9]+\/)([0-9]{1,2})`)
	// Matches IP/CIDR
	// Groups: 1.  [d.d.d.d]/dd
	//         2.   d.d.d.d/[dd]

	r4 := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+$`)
	// Matches single IP

	for i, scope := range scopes {
		scanner := bufio.NewScanner(strings.NewReader(scope))
		exclude = false // reset flag on each run

		fmt.Printf("%s Grabbing targets from %s \n", color.FgGray.Text("[-]"), source[i])

		// Scan each line in scope, identify and add target URI's to struct
		for scanner.Scan() {
			m1 := r1.FindAllStringSubmatch(scanner.Text(), -1)
			m2 := r2.FindAllStringSubmatch(scanner.Text(), -1)
			m3 := r3.FindAllString(scanner.Text(), -1)
			m4 := r4.FindAllString(scanner.Text(), -1)

			// check exclude
			if strings.Contains(scanner.Text(), exTag) {
				exclude = true
			} else if strings.Contains(scanner.Text(), incTag) {
				exclude = false
			}

			// Single IP
			if m4 != nil {
				m.Counter++
				printFound(m4[0], exclude, silent)
				if exclude != true {
					m.Includes = append(m.Includes, m4)
				} else {
					m.Excludes = append(m.Excludes, m4)
				}
			}

			// IP/CIDR
			if m3 != nil {
				for _, arr := range m3 {
					// not interested in those ending with '.'
					if strings.HasSuffix(arr, ".") {
						continue
					}
					hosts, err := hostsFromCIDR(arr)
					if err != nil {
						log.Fatalf("\n%s Failed to parse IP/CIDR: %s", color.FgRed.Text("[!]"), arr)
					} else {
						m.Counter++
						printFound(arr, exclude, silent)
					}
					if exclude != true {
						m.Includes = append(m.Includes, hosts)
					} else {
						m.Excludes = append(m.Excludes, hosts)
					}
				}

				// IP-Range
			} else if m2 != nil {
				for _, arr := range m2 {
					// not interested in those ending with '.'
					if strings.HasSuffix(arr[0], ".") {
						continue
					}

					hosts, err1, err2 := hostsFromRange(arr)
					if err1 != nil || err2 != nil {
						log.Fatalf("\n%s Failed to parse IP-range: %s", color.FgRed.Text("[!]"), arr[0])
					} else {
						m.Counter++
						printFound(arr[0], exclude, silent)

						if exclude != true {
							m.Includes = append(m.Includes, hosts)
						} else {
							m.Excludes = append(m.Excludes, hosts)

							}
						}
					}

				// anything else
			} else if m1 != nil {
				// not interested in those ending with '.'
				for _, arr := range m1 {
					if strings.HasSuffix(arr[0], ".") {
						continue
					}
					m.Counter++
					printFound(arr[0], exclude, silent)
					if exclude != true {
						m.Includes = append(m.Includes, arr)
					} else {
						m.Excludes = append(m.Excludes, arr)
					}
				}
			}
		}

		if m.Counter == 0 && bbaas {
		} else if m.Counter == 0 && !bbaas {
			fmt.Printf("%s No targets found in %s\n", color.FgRed.Text("[!]"), source[i])
		}
	}
	return m
}

// getAnswer takes question and prompts user for y/n input
// returns answer
func getAnswer(question string) string {
	var answer string
	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("\n%s %s (%s/n): ", color.FgYellow.Text("[?]"), question, color.Bold.Text("Y"))
		answer, _ = reader.ReadString('\n')
		answer = strings.ToUpper((strings.TrimSuffix(answer, "\n")))
		if answer == "Y" || answer == "N" || answer == "" {
			return answer
		}
	}
}

// isIP returns bool depending on whether string matches IP address
func isIP(s string) bool {
	re := regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)
	if re.MatchString(s) {
		return true
	}
	return false
}
// prints item in color depending on whether it is part of include or exclude
func printFound(item string, exclude bool, silent bool) {
	if exclude == true {
		if !silent {
			fmt.Println(color.FgRed.Text(" -  " + item))
		}
	} else {
		if !silent {
			fmt.Println(color.FgGreen.Text(" +  " + item))
		}
	}
}

// hostsFromRange takes a m2 slice containing IP-range substrings
// converts range to a list of hosts and returns this
func hostsFromRange(m []string) ([]string, error, error) {
	ip := m[2] // [192.168.0].1-255

	start, err1 := strconv.Atoi(m[3]) // 192.168.0.(1)-255
	end, err2 := strconv.Atoi(m[4])   // 192.168.(0).(1)-(255)
	var ips []string

	for i := start; i <= end; i++ {
		ip := ip + strconv.Itoa(i)
		ips = append(ips, ip)
	}
	return ips, err1, err2
}

// hostsFromCIDR takes a m3 slice containing IP/CIDR substrings
// converts CIDR to list of hosts and returns this
func hostsFromCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	var ips []string
	// we only want the IP
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ips = append(ips, ip.String())
	}
	return ips[1 : len(ips)-1], nil
}

// inc increments host in IP range
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
