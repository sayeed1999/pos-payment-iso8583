// Command server is the acquirer-side host: it listens for POS terminals
// speaking framed ISO 8583 over TCP.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"titp-poc/internal/application"
	"titp-poc/internal/domain"
	"titp-poc/internal/handlers/iso8583"
)

func main() {
	addr := flag.String("addr", ":8583", "TCP listen address")
	raw := flag.Bool("raw", false, "also print each raw frame (UNMASKED card data, PoC only)")
	flag.Parse()

	l, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	s := iso8583.New(application.NewHost())
	s.ShowRaw = *raw

	fmt.Printf("ISO 8583 host listening on %s\n\n", l.Addr())
	fmt.Println("Test cards:")
	fmt.Println("  4111111111111111  Visa        -> approve")
	fmt.Println("  5555555555554444  Mastercard  -> approve")
	fmt.Println("  4000000000000002  Visa        -> 05 do not honour")
	fmt.Println("  4000000000000341  Visa        -> approve, but the reply is lost (terminal must reverse)")
	fmt.Printf("  PIN %s is correct, any other PIN -> 55\n\n", domain.DemoPIN)

	log.Fatal(s.Serve(l))
}
