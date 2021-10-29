package main

import (
	"context"
	"flag"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type dnsMistake struct{}

func (*dnsMistake) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := dns.Msg{}
	msg.SetReply(r)
	switch r.Question[0].Qtype {
	case dns.TypeA:
		msg.Authoritative = true
		domain := msg.Question[0].Name
		address, ok := haveIT(domain)
		if ok {
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: domain, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(address),
			})
		}
	}
	w.WriteMsg(&msg)
}

var (
	upStrdns *string
	upStrcon *string
)

func flushCH(name string) {
	for {
		<-flush
		log.Printf("dropping memory cache .... %v recored", len(dataCH))
		dataCH = make(map[string]string)
		fileCheck(name)

	}

}

func askUpstr(s string) (string, bool) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Millisecond * time.Duration(10000),
			}
			return d.DialContext(ctx, *upStrcon, *upStrdns)
		},
	}

	ip, err := r.LookupHost(context.Background(), s)
	if err != nil {
		return "err", false
	}
	return ip[0], true

}

const regIPPORT string = "^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5]):()([1-9]|[1-5]?[0-9]{2,4}|6[1-4][0-9]{3}|65[1-4][0-9]{2}|655[1-2][0-9]|6553[1-5])$"
const regTCPUDP string = "^(tcp|udp)$"
const ipONLY string = "^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])$"

var dataCH = make(map[string]string)
var datamx = &sync.Mutex{}
var fakeAdd *string

func timeCh() {
	for {
		time.Sleep(time.Second*time.Duration(rand.Intn(120)) + 90)
		flush <- struct{}{}
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	upStrdns = flag.String("upaddr", "1.1.1.1:53", "upsteam dns server to connect <ipaddr:port>")
	listenAddr := flag.String("loaddr", "0.0.0.0:53", "dns server listen address <ipaddr:port>")
	upStrcon = flag.String("upconn", "udp", "upsteam dns connection type <udp|tcp>")
	listenCon := flag.String("loconn", "udp", "dns server connection type <udp|tcp>")
	fakeAdd = flag.String("fakeadd", "127.0.0.1", "an ip to send back for filtered domains <ipaddr>")
	filter := flag.String("filter", "noacc.txt", "filtered domains <filename> - regex is supported")

	flag.Parse()
	if match, _ := regexp.MatchString(regIPPORT, *upStrdns); !match {
		flag.Usage()
		os.Exit(1)
	}
	if match, _ := regexp.MatchString(ipONLY, *fakeAdd); !match {
		flag.Usage()
		os.Exit(1)
	}
	if match, _ := regexp.MatchString(regIPPORT, *listenAddr); !match {
		flag.Usage()
		os.Exit(1)
	}
	if match, _ := regexp.MatchString(regTCPUDP, *upStrcon); !match {
		flag.Usage()
		os.Exit(1)
	}
	if match, _ := regexp.MatchString(regTCPUDP, *listenCon); !match {
		flag.Usage()
		os.Exit(1)
	}

	fileCheck(*filter)
	go flushCH(*filter)
	go timeCh()
	go memCH()
	srv := &dns.Server{Addr: *listenAddr, Net: *listenCon}
	srv.Handler = &dnsMistake{}
	log.Printf("upstream dns: %v connection %v", *upStrdns, *upStrcon)
	log.Printf("dns listen on: %v connection %v", *listenAddr, *listenCon)
	log.Printf("load banned domains from: %v", *filter)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("fatal: failed to set udp|tcp listener %s\n", err.Error())
	}
}

func fileCheck(name string) {
	content, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal("fatal: can not access or create the file", err.Error())
	}
	ready, err := ioutil.ReadAll(content)
	if err != nil {
		log.Fatal("fatal: unknow error", err.Error())
	}
	stringArr := strings.Fields(string(ready))
	for _, v := range stringArr {
		dataCH[v] = *fakeAdd
	}

}

var flush = make(chan struct{})

func haveIT(domain string) (string, bool) {
	for k, v := range dataCH {
		if ok, _ := regexp.MatchString(k, domain); ok {
			return v, true
		}
	}

	if addr, ok := askUpstr(domain); ok {
		datamx.Lock()
		dataCH[domain] = addr
		datamx.Unlock()
		return addr, true
	}
	return "err", false
}

func memCH() {
	for {
		time.Sleep(time.Millisecond * 100)
		if len(dataCH) > 900000 {
			flush <- struct{}{}
		}

	}
}
