package edgar

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

var (
	baseURL   string = "https://www.sec.gov/"
	cikURL    string = "https://www.sec.gov/cgi-bin/browse-edgar?action=getcompany&output=xml&CIK=%s"
	queryURL  string = "cgi-bin/browse-edgar?action=getcompany&CIK=%s&type=%s&dateb=&owner=exclude&count=10"
	searchURL string = baseURL + queryURL
)

func createQueryURL(symbol string, docType FilingType) string {
	return fmt.Sprintf(searchURL, symbol, docType)
}

func getPage(url string) io.ReadCloser {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("Query to SEC page ", url, "failed: ", err)
		return nil
	}
	return resp.Body
}

func getCompanyCIK(ticker string) string {
	fmt.Println("getting company CIK")
	var t bool
	if strings.Contains(ticker, " ") {
		t = true
	} else {
		url1 := fmt.Sprintf(cikURL, ticker)
		r := getPage(url1)
		rb, _ := ioutil.ReadAll(r)
		t = strings.Contains(string(rb),"No matching Ticker Symbol.")
	}
	switch {
	case t == false:
		url1 := fmt.Sprintf(cikURL, ticker)
		r2  := getPage(url1)
		if cik, err := cikPageParser(r2); err == nil {
			return cik
		}
	case t == true:
		r := postPage(backupCIK, ticker)
		if r != nil {
			if cik, err := cikPostPageParser(r); err == nil {
				fmt.Println(cik)
				return cik
			}
		}
	default:
		fmt.Println("in default")
	   return ""
	}
	return ""
}

// getFilingLinks gets the links for filings of a given type of filing 10K/10Q..
func getFilingLinks(ticker string, fileType FilingType) map[string]string {
	url := createQueryURL(ticker, fileType)
	resp := getPage(url)
	if resp == nil {
		log.Println("No response on the query for docs")
		return nil
	}
	defer resp.Close()
	return queryPageParser(resp, fileType)

}

//Get all the docs pages based on the filing type
//Returns a map:
// key=Document type ex.Cash flow statement
// Value = link to that that sheet
func getFilingDocs(url string, fileType FilingType) map[filingDocType]string {
	url = baseURL + url
	resp := getPage(url)
	if resp == nil {
		return nil
	}
	defer resp.Close()
	return filingPageParser(resp, fileType)
}

// getFinancialData gets the data from all the filing docs and places it in
// a financial report
func getFinancialData(url string, fileType FilingType) (*financialReport, error) {
	docs := getFilingDocs(url, fileType)
	return parseMappedReports(docs, fileType)
}
