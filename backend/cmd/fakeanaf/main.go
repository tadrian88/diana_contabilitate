// Command fakeanaf is a local, synthetic ANAF-shaped server for development.
// It contains no production/customer data and must never be used as a validator.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"time"
)

func main() {
	address := flag.String("address", "127.0.0.1:8090", "listen address")
	buyerCUI := flag.String("buyer-cui", "RO990002", "synthetic buyer CUI")
	flag.Parse()
	zipBytes := invoiceZIP(*buyerCUI)
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		redirectURI := r.URL.Query().Get("redirect_uri")
		if !validRedirectURI(redirectURI) {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = authorizationPage.Execute(w, map[string]string{
			"RedirectURI": redirectURI,
			"State":       r.URL.Query().Get("state"),
			"BuyerCUI":    *buyerCUI,
		})
	})
	mux.HandleFunc("/authorize/complete", func(w http.ResponseWriter, r *http.Request) {
		redirect, err := url.Parse(r.URL.Query().Get("redirect_uri"))
		if err != nil || redirect.Scheme == "" || redirect.Host == "" {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
		query := redirect.Query()
		query.Set("code", "fake-authorization-code")
		query.Set("state", r.URL.Query().Get("state"))
		redirect.RawQuery = query.Encode()
		http.Redirect(w, r, redirect.String(), http.StatusSeeOther)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-access", "refresh_token": "fake-refresh", "expires_in": 3600, "refresh_expires_in": 7200})
	})
	mux.HandleFunc("/listaMesajePaginatieFactura", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"mesaje": []map[string]string{{"id": "70001", "id_solicitare": "60001", "tip": "FACTURA PRIMITA", "data_creare": "202609141200"}}, "numar_total_pagini": 1, "cui": *buyerCUI})
	})
	mux.HandleFunc("/descarcare", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBytes)
	})
	server := &http.Server{Addr: *address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("synthetic fake ANAF listening on %s", *address)
	log.Fatal(server.ListenAndServe())
}

func validRedirectURI(value string) bool {
	redirect, err := url.Parse(value)
	return err == nil && (redirect.Scheme == "http" || redirect.Scheme == "https") && redirect.Host != ""
}

var authorizationPage = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="ro"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ANAF simulată</title>
<style>body{font-family:system-ui,sans-serif;background:#f6f7fb;color:#171047;margin:0;padding:3rem}.card{max-width:42rem;margin:auto;background:white;border:1px solid #ddd;border-radius:1rem;padding:2rem}.meta{background:#f2f4f8;border-radius:.75rem;padding:1rem;margin:1.5rem 0}button{background:#24136f;color:white;border:0;border-radius:.6rem;padding:.8rem 1.1rem;font-weight:700;cursor:pointer}</style></head>
<body><main class="card"><p>Simulator local — fără certificat real</p><h1>Autorizare ANAF simulată</h1><p>În serviciul real, browserul selectează certificatul calificat înainte de pagina de consimțământ ANAF. Certificatul și PIN-ul nu ajung la Diana.</p>
<div class="meta"><strong>Certificat sintetic calificat</strong><br>Autoritate: Furnizor Test<br>CUI autorizat: {{.BuyerCUI}}</div>
<form method="get" action="/authorize/complete"><input type="hidden" name="redirect_uri" value="{{.RedirectURI}}"><input type="hidden" name="state" value="{{.State}}"><button type="submit">Autorizează cu certificatul sintetic</button></form></main></body></html>`))

func invoiceZIP(buyerCUI string) []byte {
	xml := fmt.Sprintf(`<Invoice><ID>FAKE-ANAF-LOCAL-1</ID><IssueDate>2026-09-14</IssueDate><DocumentCurrencyCode>RON</DocumentCurrencyCode><AccountingSupplierParty><Party><PartyLegalEntity><RegistrationName>Furnizor Sintetic</RegistrationName></PartyLegalEntity><PartyTaxScheme><CompanyID>RO990003</CompanyID></PartyTaxScheme></Party></AccountingSupplierParty><AccountingCustomerParty><Party><PartyTaxScheme><CompanyID>%s</CompanyID></PartyTaxScheme></Party></AccountingCustomerParty><LegalMonetaryTotal><TaxInclusiveAmount>119</TaxInclusiveAmount></LegalMonetaryTotal><InvoiceLine><ID>1</ID><InvoicedQuantity unitCode="H87">1</InvoicedQuantity><LineExtensionAmount>100</LineExtensionAmount><Item><Name>Linie sintetică</Name><ClassifiedTaxCategory><Percent>19</Percent></ClassifiedTaxCategory></Item><Price><PriceAmount>100</PriceAmount></Price></InvoiceLine></Invoice>`, buyerCUI)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, _ := writer.Create("invoice.xml")
	_, _ = entry.Write([]byte(xml))
	_ = writer.Close()
	return buffer.Bytes()
}
