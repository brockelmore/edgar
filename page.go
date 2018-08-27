package edgar

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

var (
	baseURL   string = "https://www.sec.gov/"
	queryURL  string = "cgi-bin/browse-edgar?action=getcompany&CIK=%s&type=%s&dateb=&owner=exclude&count=10"
	searchURL string = baseURL + queryURL
)

func createQueryURL(symbol string, docType FilingType) string {
	return fmt.Sprintf(searchURL, symbol, docType)
}

func getPage(url string) io.ReadCloser {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal("Query to SEC page failed: ", err)
		return nil
	}
	return resp.Body
}

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

func getFinancialData(url string, fileType FilingType) *financialReport {

	var err error

	docs := getFilingDocs(url, fileType)

	fr := new(financialReport)

	fr.DocType = fileType
	for key, val := range docs {
		log.Println("Getting: ", key, val)
		url := baseURL + val
		page := getPage(url)
		if page == nil {
			log.Fatal("Did not find the doc page" + val)
		}
		defer page.Close()

		switch key {
		case filingDocBS:
			fr.Bs, err = getBSData(page)
			if err != nil {
				log.Fatal("Failed to get the Balance sheet doc: ", err)
			}
		case filingDocCF:
			fr.Cf, err = getCfData(page)
			if err != nil {
				log.Fatal("Failed to get the cash flow doc: ", err)
			}
		case filingDocEN:
			fr.Entity, err = getEntityData(page)
			if err != nil {
				log.Fatal("Failed to get the Entity sheet doc: ", err)
			}
		case filingDocOps:
			fr.Ops, err = getOpsData(page)
			if err != nil {
				log.Fatal("Failed to get the operations sheet doc ", err)
			}

		}
	}
	return fr
}