package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type Request struct {
	method  string
	url     string
	values  url.Values
	headers map[string]string
}

func main() {

	flag.Parse()

	exchangeURL := flag.Arg(0)
	userid := flag.Arg(1)
	password := flag.Arg(2)
	fmt.Println("URL", exchangeURL)
	Host := strings.Replace(exchangeURL, "http://", "", 1)
	Host = strings.Replace(Host, "/wordpress/", "", 1)
	fmt.Println("Host", Host)

	if exchangeURL == "" {
		fmt.Println("You should add URL")
		os.Exit(2)
	}

	req := &Request{
		method: "GET",
		url:    exchangeURL,
		values: url.Values{},
		headers: map[string]string{
			"Host":       Host,
			"User-Agent": "Mozilla/5.0 (Windows NT 6.3; WOW64; Trident/7.0; MAFSJS; rv:11.0) like Gecko)",
		},
	}

	//Requestを投げる
	response, err := req.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	nextPage, err := getMyAccount(response)
	if err != nil {
		fmt.Println("getMyAccount error: ", err)
		os.Exit(2)
	}

	referer := req.url
	req.url = nextPage
	req.headers["Referer"] = referer

	response, err = req.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	formTo, redirect, err := checkLoginForm(response)
	if err != nil {
		fmt.Println("checkLoginForm error: ", err)
		os.Exit(2)
	}

	referer = req.url

	postReq := &Request{
		method: "POST",
		url:    exchangeURL,
		values: url.Values{},
		headers: map[string]string{
			"Host":                      Host,
			"User-Agent":                "Mozilla/5.0 (Windows NT 6.3; WOW64; Trident/7.0; MAFSJS; rv:11.0) like Gecko)",
			"Referer":                   referer,
			"Connection":                "keep-alive",
			"Accept-Language":           "ja,en-US;q=0.7,en;q=0.3",
			"Accept-Encoding":           "gzip, deflate",
			"Accept":                    "text/html,application/xhtml+xm…ml;q=0.9,image/webp,*/*;q=0.8",
			"Upgrade-Insecure-Requests": "1",
			"Content-Type":              "application/x-www-form-urlencoded",
		},
	}

	loginReq := postReq
	loginReq.url = formTo
	loginReq.values.Set("log", userid)
	loginReq.values.Add("pwd", password)
	loginReq.values.Add("wp-submit", "ログイン")
	loginReq.values.Add("redirect_to", redirect)
	loginReq.values.Add("testcookie", "1")

	response, err = loginReq.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	var cookie string

	for k, v := range response.Header {

		if k == "Set-Cookie" {
			for _, x := range v {
				if regexp.MustCompile("wordpress_logged_in_*").Match([]byte(x)) {
					cookie = x
				}
			}
		}
	}

	if cookie != "" {
		rep := regexp.MustCompile(`;\spath.*$`)
		cookie = rep.ReplaceAllString(cookie, "")
	}

	req = &Request{
		method: "GET",
		url:    nextPage,
		values: url.Values{},
		headers: map[string]string{
			"Host":       Host,
			"User-Agent": "Mozilla/5.0 (Windows NT 6.3; WOW64; Trident/7.0; MAFSJS; rv:11.0) like Gecko)",
			"Cookie":     "wordpress_test_cookie=WP+Cookie+check; " + cookie,
			"Connection": "keep-alive",
		},
	}

	buyDoc, err := req.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	sellDoc, err := req.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	buyOrderURL, err := getFirstBuyOrder(buyDoc)

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	fmt.Println("BuyOrder", buyOrderURL)

	req.url = buyOrderURL
	buyStepDoc, err := req.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	compBuyURL, purchaseID, err := comfirmBuyOrder(buyStepDoc)

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	fmt.Println("url", compBuyURL, ":", "purchase_ID", purchaseID)

	compSellReq := postReq
	compSellReq.url = compBuyURL
	compSellReq.headers["referer"] = buyOrderURL
	compSellReq.headers["Cookie"] = "wordpress_test_cookie=WP+Cookie+check; " + cookie
	compSellReq.values.Set("purchase_id", purchaseID)
	compSellReq.values.Set("wp-submit", "売却")

	response, err = compSellReq.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	if chechHack(response) == false {
		fmt.Println("Can not Accessed Go-Ethereum")
		os.Exit(2)
	}

	fmt.Println("Complete BuyOrder")

	sellOrderURL, err := getFirstSellOrder(sellDoc)

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	fmt.Println("SellOrder", sellOrderURL)

	req.url = sellOrderURL
	sellStepDoc, err := req.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	compSellURL, sellID, err := comfirmSellOrder(sellStepDoc)

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	fmt.Println("url", compSellURL, ":", "sellID", sellID)

	compBuyReq := postReq
	compBuyReq.url = compSellURL
	compBuyReq.headers["referer"] = sellOrderURL
	compBuyReq.headers["Cookie"] = "wordpress_test_cookie=WP+Cookie+check; " + cookie
	compBuyReq.values.Set("purchase_id", sellID)
	compBuyReq.values.Set("wp-submit", "購入")

	response, err = compSellReq.Request()

	if err != nil {
		fmt.Println("Request error: ", err)
		os.Exit(2)
	}

	if chechHack(response) == false {
		fmt.Println("Can not Accessed Go-Ethereum")
		os.Exit(2)
	}

	fmt.Println("Complete SellOrder")

}

//Request Function is check and execute Request and return Response
func (param Request) Request() (*http.Response, error) {

	if param.method == "GET" {

		response, err := param.getRequest()

		if err != nil {
			return nil, err
		}

		return response, nil

	} else if param.method == "POST" {
		response, err := param.postRequest()

		if err != nil {
			return nil, err
		}

		return response, nil
	}

	return nil, errors.New("unknown http method")
}

func (param *Request) postRequest() (*http.Response, error) {
	client := &http.Client{Timeout: time.Duration(10) * time.Second}
	req, err := http.NewRequest(param.method, param.url, strings.NewReader(param.values.Encode()))

	if err != nil {
		return nil, err
	}

	if param.headers != nil {
		for key, value := range param.headers {
			req.Header.Add(key, value)
		}
	}

	body, _ := ioutil.ReadAll(req.Body)
	param.headers["Content-Length"] = string(len(body))

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	response := resp

	return response, nil
}

func (param *Request) getRequest() (*http.Response, error) {

	req, err := http.NewRequest(param.method, param.url, nil)
	client := http.Client{Timeout: time.Duration(10) * time.Second}

	if err != nil {
		return nil, err
	}

	if param.headers != nil {
		for key, value := range param.headers {
			req.Header.Add(key, value)
		}
	}

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	return resp, nil

}

// レスポンスをNewDocumentFromResponseに渡してドキュメントを得る
func getMyAccount(resp *http.Response) (string, error) {

	var result string

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return "Can not recieved Response", err
	}
	doc.Find("a").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		url, _ := s.Attr("href")
		t := s.Text()
		if t == "My account" {
			result = url
			return false
		}
		return true
	})

	if result != "" {
		return result, nil
	}

	return "", errors.New("Can not find `My account` by Element")

}

// レスポンスをNewDocumentFromResponseに渡してログインフォームを得る
func checkLoginForm(resp *http.Response) (string, string, error) {

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return "", "", err
	}

	action, _ := doc.Find("form").First().Attr("action")

	if action == "" {
		return "", "", errors.New("Can not find `form#action` by Element")
	}

	redirect, _ := doc.Find("input[type=hidden]").First().Attr("value")
	if redirect == "" {
		return "", "", errors.New("Can not find `redirect_to` by Element")
	}
	return action, redirect, nil
}

// レスポンスをNewDocumentFromResponseに渡して最初の買い注文を得る
func getFirstBuyOrder(resp *http.Response) (string, error) {

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return "", err
	}

	selection := doc.Find("table.buy")

	buyorder, _ := selection.Find("a").Attr("href")

	if buyorder == "" {
		return "", errors.New("Can not find buyorder")
	}

	return buyorder, nil
}

// レスポンスをNewDocumentFromResponseに渡して最初の売り注文を得る
func getFirstSellOrder(resp *http.Response) (string, error) {

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return "", err
	}

	selection := doc.Find("table.sell")

	sellorder, _ := selection.Find("a").Attr("href")

	if sellorder == "" {
		return "", errors.New("Can not find sellorder")
	}

	return sellorder, nil
}

// 買い注文の処理を完了させる
func comfirmBuyOrder(resp *http.Response) (string, string, error) {

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return "", "", err
	}

	url, _ := doc.Find("div#main").Find("form").Attr("action")

	if url == "" {
		return "", "", errors.New("Can not find postToURL")
	}

	sellID, _ := doc.Find("div#main").Find("input").Attr("value")

	if sellID == "" {
		return "", "", errors.New("Can not find SellID")
	}

	return url, sellID, nil
}

// 売り注文の処理を完了させる
func comfirmSellOrder(resp *http.Response) (string, string, error) {

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return "", "", err
	}

	url, _ := doc.Find("div#main").Find("form").Attr("action")

	if url == "" {
		return "", "", errors.New("Can not find postToURL")
	}

	sellID, _ := doc.Find("div#main").Find("input").Attr("value")

	if sellID == "" {
		return "", "", errors.New("Can not find BuyID")
	}

	return url, sellID, nil
}

func chechHack(resp *http.Response) bool {
	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return false
	}
	if strings.Contains(doc.Text(), "Warning") {
		return false
	}
	return true
}

// Send to Elastic from score
