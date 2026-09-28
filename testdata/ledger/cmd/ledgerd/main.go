// Command ledgerd serves quotes.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/acme/ledger/internal/httpapi"
	"github.com/acme/ledger/internal/store"
)

func main() {
	if err := run(context.Background(), addr()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, listen string) error {
	st, err := store.Open(ctx, store.ConfigFromEnv())
	if err != nil {
		return err
	}
	defer st.Close(ctx)
	log.Printf("ledgerd listening on %s", listen)
	return http.ListenAndServe(listen, httpapi.New(st).Routes())
}

func addr() string {
	if a := os.Getenv("LEDGER_ADDR"); a != "" {
		return a
	}
	return "127.0.0.1:8080"
}
