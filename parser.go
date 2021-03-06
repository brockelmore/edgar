package edgar

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"bytes"
	"regexp"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

func parseCikAndDocId(url string) (string, string) {
	var s1 string
	var d1, d2, d3, d4 int
	fmt.Sscanf(url, "/cgi-bin/viewer?action=view&cik=%d&accession_number=%d-%d-%d%s", &d1, &d2, &d3, &d4, &s1)
	cik := fmt.Sprintf("%d", d1)
	an := fmt.Sprintf("%010d%d%d", d2, d3, d4)
	return cik, an
}

/*
  This is the parsing of query page where we get the list of filings of a given types
  ex: https://www.sec.gov/cgi-bin/browse-edgar?CIK=AAPL&owner=exclude&action=getcompany&type=10-Q&count=1&dateb=
  Assumptions of the parser:
  - There is interactive data available and there is a button that allows the user to click it
  - Since it is a link the tag will be a hyperlink with a button with the id=interactiveDataBtn
  - The actual link is the href attribute in the "a" token just before the id attribute
*/
func queryPageParser(page io.Reader, docType FilingType) map[string]string {

	filingInfo := make(map[string]string)

	z := html.NewTokenizer(page)

	data, err := parseTableRow(z, true)
	for err == nil {
		//This check for filing type will drop AMEND filings
		if len(data) == 5 && data[0] == string(docType) {
			//Drop filings before 2010
			year := getYear(data[3])
			if year >= thresholdYear {
				filingInfo[data[3]] = data[1]
			}
		}
		data, err = parseTableRow(z, true)
	}
	return filingInfo
}

func cikPageParser(page io.Reader) (string, error) {
	z := html.NewTokenizer(page)
	token := z.Token()
	for !(token.Data == "cik" && token.Type == html.StartTagToken) {
		tt := z.Next()
		if tt == html.ErrorToken {
			return "", errors.New("Could not find the CIK")
		}
		token = z.Token()
	}
	for !(token.Data == "cik" && token.Type == html.EndTagToken) {
		if token.Type == html.TextToken {
			str := strings.TrimSpace(token.String())
			if len(str) > 0 {
				return str, nil
			}
		}
		z.Next()
		token = z.Token()
	}
	return "", errors.New("Could not find the CIK")
}
func cikPostPageParser(page io.Reader) (string, error) {
	doc, _ := html.Parse(page)
	r := regexp.MustCompile(`CIK=[+]?\d{2,}$`)
	var CIK string
	var f func(*html.Node)
	f = func(n *html.Node) {
	    if n.Type == html.ElementNode && n.Data == "a" {
	        for _, a := range n.Attr {
	            if a.Key == "href" {
			m := r.FindStringSubmatch(a.Val)
			if len(m) > 0 {
				CIK = strings.Split(m[0], "=")[1]
			}
	                break
	            }
	        }
	    }
	    for c := n.FirstChild; c != nil; c = c.NextSibling {
	        f(c)
	    }
	}
	f(doc)
	if CIK != "" {
		for len(CIK) < 10 {
			CIK = "0" + CIK
		}
		return CIK, nil
	}
	return CIK, errors.New("Could not find CIK")
}
/*
  The filing page parser
  - The top of the page has a list of reports.
  - Get all the reports (link to all the reports) and put it in an array
  - The Accordian on the side of the page identifies what each report is
  - Get the text of the accordian and map the type of the report to the report
  - Create a map of the report to report link
*/
func filingPageParser(page io.Reader, fileType FilingType) map[filingDocType]string {
	var filingLinks []string
	r := bufio.NewReader(page)
	s, e := r.ReadString('\n')

	for e == nil {
		//Get the number of reports available
		if strings.Contains(s, "var reports") == true {
			s1 := strings.Split(s, "(")
			s2 := strings.Split(s1[1], ")")
			cnt, _ := strconv.Atoi(s2[0])

			//cnt-1 because we skip the 'all' in the list
			for i := 0; i < cnt-1; i++ {
				s, e = r.ReadString('\n')
				s1 := strings.Split(s, " = ")
				s2 := strings.Split(s1[1], ";")
				s3 := strings.Trim(s2[0], "\"")
				s4 := strings.Split(s3, ".")
				s5 := s3
				//Sometimes the report is listed as an xml file??
				if s4[1] == "xml" {
					s5 = s4[0] + ".htm"
				}
				if !strings.Contains(s5, "htm") {
					panic("Dont know this type of report")
				}
				filingLinks = append(filingLinks, s5)
			}

			break
		}
		s, e = r.ReadString('\n')

	}

	docs := mapReports(page, filingLinks)
	return docs

}

func parseTableData(z *html.Tokenizer, parseHref bool) string {
	token := z.Token()

	if token.Type != html.StartTagToken && token.Data != "td" {
		log.Fatal("Tokenizer passed incorrectly to parseTableData")
		return ""
	}

	for !(token.Data == "td" && token.Type == html.EndTagToken) {
		if token.Type == html.ErrorToken {
			break
		}

		if parseHref && token.Data == "a" && token.Type == html.StartTagToken {
			str := parseHyperLinkTag(z, token)
			if len(str) > 0 {
				return str
			}
		} else {
			if token.Type == html.TextToken {
				str := strings.TrimSpace(token.String())
				if len(str) > 0 {
					return str
				}
			}
		}
		//Going for the end of the td tag
		z.Next()
		token = z.Token()
	}
	return ""
}

func parseTableRow(z *html.Tokenizer, parseHref bool) ([]string, error) {
	var retData []string
	//Get the current token
	token := z.Token()

	//Check if this is really a table row
	for !(token.Type == html.StartTagToken && token.Data == "tr") {
		tt := z.Next()
		if tt == html.ErrorToken {
			return nil, errors.New("Done with parsing")
		}
		token = z.Token()
	}
	//Till the end of the row collect data from each data block
	for !(token.Data == "tr" && token.Type == html.EndTagToken) {

		if token.Type == html.ErrorToken {
			return nil, errors.New("Done with parsing")
		}
		if token.Data == "td" && token.Type == html.StartTagToken {
			parseFlag := parseHref
			//If the data is a number class just get the text = number
			for _, a := range token.Attr {
				if a.Key == "class" && (a.Val == "nump" || a.Val == "num") {
					parseFlag = false
				}
			}
			str := parseTableData(z, parseFlag)
			if len(str) > 0 {
				retData = append(retData, str)
			}
		}
		z.Next()
		token = z.Token()
	}

	return retData, nil
}

var reqHyperLinks = map[string]bool{
	"interactiveDataBtn": true,
}

func parseHyperLinkTag(z *html.Tokenizer, token html.Token) string {
	var href string
	var onclick string
	var id string
	for _, a := range token.Attr {
		switch a.Key {
		case "id":
			id = a.Val
		case "href":
			href = a.Val
		case "onclick":
			onclick = a.Val
			if str, err := getFinDataXBRLTag(onclick); err == nil {
				return str
			}
		}
	}

	text := ""
	//Finish up the hyperlink
	for !(token.Data == "a" && token.Type == html.EndTagToken) {
		/*
			if token.Type == html.TextToken {
				str := strings.TrimSpace(token.String())
				if len(str) > 0 {
					text = str
				}
			}
		*/
		z.Next()
		token = z.Token()
	}

	if _, ok := reqHyperLinks[id]; ok {
		return href
	}

	return text
}

func parseTableTitle(z *html.Tokenizer) []string {

	var strs []string
	token := z.Token()

	if token.Type != html.StartTagToken && token.Data != "th" {
		log.Fatal("Tokenizer passed incorrectly to parseTableData")
		return strs
	}

	for !(token.Data == "th" && token.Type == html.EndTagToken) {
		if token.Type == html.ErrorToken {
			break
		}

		if token.Type == html.TextToken {
			str := strings.TrimSpace(token.String())
			if len(str) > 0 {
				strs = append(strs, str)
			}
		}
		//Going for the end of the td tag
		z.Next()
		token = z.Token()
	}
	return strs
}

func parseTableHeading(z *html.Tokenizer) ([]string, error) {
	var retData []string
	//Get the current token
	token := z.Token()

	//Check if this is really a table row
	for !(token.Type == html.StartTagToken && token.Data == "tr") {
		tt := z.Next()
		if tt == html.ErrorToken {
			return nil, errors.New("Done with parsing")
		}
		token = z.Token()
	}

	//Till the end of the row collect data from each data block
	for !(token.Data == "tr" && token.Type == html.EndTagToken) {

		if token.Type == html.ErrorToken {
			return nil, errors.New("Done with parsing")
		}
		if token.Data == "th" && token.Type == html.StartTagToken {
			str := parseTableTitle(z)
			if len(str) > 0 {
				retData = append(retData, str...)
			}
		}
		z.Next()
		token = z.Token()
	}

	return retData, nil
}

// func parseFilingScale(z *html.Tokenizer, t filingDocType) map[scaleEntity]scaleFactor {
// 	scales := make(map[scaleEntity]scaleFactor)
// 	data, err := parseTableHeading(z)
// 	if err == nil {
// 		if len(data) > 0 {
// 			scales = filingScale(data, t)
// 		}
// 	}
// 	return scales
func parseFilingScale(page io.Reader, t filingDocType) map[scaleEntity]scaleFactor {
	ret := make(map[scaleEntity]scaleFactor)
	if t == filingDocEN {
		ret[scaleEntityShares] = scaleNone
	} else {
		ret[scaleEntityShares] = scaleMillion
	}
	ret[scaleEntityMoney] = scaleNone
	ret[scaleEntityPerShare] = scaleNone

	doc, err := goquery.NewDocumentFromReader(page)
	if err != nil {
	  log.Fatal(err)
	}
	doc.Find("body table tr th strong").Each(func(i int, sq *goquery.Selection){
	    s := strings.ToLower(sq.Text())
	    parts := strings.Split(s, ",")
	    for _, part := range parts {
	      if strings.Contains(part, "share") {
			// Share scale
			if strings.Contains(part, "thousand") {
				ret[scaleEntityShares] = scaleThousand
			} else if strings.Contains(part, "million") {
				ret[scaleEntityShares] = scaleMillion
			}
		} else if strings.Contains(part, "$") || strings.Contains(part, "usd") {
			//Money scale
			if strings.Contains(part, "thousand") {
				ret[scaleEntityMoney] = scaleThousand
			} else if strings.Contains(part, "billion") {
				ret[scaleEntityMoney] = scaleBillion
			} else if strings.Contains(part, "million") {
				ret[scaleEntityMoney] = scaleMillion
			}
		}
	  }
	  })
	return ret
}

/*
	This function takes any report filed under a company and looks
	for XBRL tags that Filing is interested in to gather and store.
	XBRL tag is mapped to a finDataType which is then used to lookup
	the passed in interface fields to see if there is a match and set
	that field
*/

func finReportParser(page io.Reader, fr *financialReport, t filingDocType) (*financialReport, error) {
	var buf bytes.Buffer
        tee := io.TeeReader(page, &buf)
	z := html.NewTokenizer(&buf)
	scales := parseFilingScale(tee, t)
	data, err := parseTableRow(z, true)
	for err == nil {
		if len(data) > 0 {
			finType := getFinDataTypeFromXBRLTag(data[0])
			if finType != finDataUnknown {
				for _, str := range data[1:] {
					if len(str) > 0 {
						if setData(fr, finType, str, scales) == nil {
							break
						}
					}
				}
			}
		}
		data, err = parseTableRow(z, true)
	}
	return fr, nil
}



func collectDataTags(page io.Reader) map[string]string {
	dataTags := make(map[string]string)
// 	dataTable.Rows = make(map[string]Row)
	doc, _ := goquery.NewDocumentFromReader(page)
	doc.Find(".report tbody td a").Each(func(i int, s *goquery.Selection) {
    		// For each item found, get the band and title
		text := s.Text()
		link, _ := s.Attr("onclick")
		if len(link) > 23 {
			if link[:23] == "top.Show.showAR( this, " {
				link = link[15:len(link)-1]
				h := strings.Split(link, " ")
				link = h[2][1:len(h[2]) - 2]
	// 			if text[len(text)] == byte(colon) {
	// 				dataTags[text[:len(text)-1]] = link
	// 			log.Printf("%s: %s\n", text[:len(text)], link)
	// 			} else {
	// 				dataTags[text] = link
	// 				log.Printf("%s: %s\n", text, link)
	// 			}
	// 			log.Println(text[len(text)-1])
				dataTags[text] = link
	// 			dataTable[text]["link"] = link
			}
		}
  	})
// 	doc.Find(".report tbody tr").Each(func(i int, s3 *goquery.Selection) {
// 		var text string
// 		var row Row
// 		s3.Find("a").Each(func(i int, s *goquery.Selection) {
// 		// For each item found, get the band and title
// 			text = s.Text()
// 			link, _ := s.Attr("onclick")
// 			if link[:23] == "top.Show.showAR( this, " {
// 				link = link[15:len(link)-1]
// 				h := strings.Split(link, " ")
// 				link = h[2][1:len(h[2]) - 2]
// // 				dataTags[text] = link
// 				row.Tag = link
// 			}
// 		})
// 		var values []string
// 		s3.Find("td").Each(func(i int, s2 *goquery.Selection) {
// 			if i == 0 {
// 			} else {
// 				if s2.Text() != " " {
// 					values = append(values, s2.Text())
// 				}
// 			}
// 		})
// 		row.Values = values
// 		dataTable.Rows[text] = row
// 	})
// 	var headers []string
// 	doc.Find(".report th").Each(func(i int, s2 *goquery.Selection) {
// 			headers = append(headers, s2.Text())
// 	})
// 	dataTable.Headers = headers
// 	log.Println(dataTags)
	return dataTags
}

// parseAllReports gets all the reports filed under a given account normalizeNumber
func parseAllReports(cik string, an string) []int {
	var reports []int
	url := "https://www.sec.gov/Archives/edgar/data/" + cik + "/" + an + "/"
	page := getPage(url)
	z := html.NewTokenizer(page)
	data, err := parseTableRow(z, false)
	for err == nil {
		var num int
		if len(data) > 0 && strings.Contains(data[0], "R") {
			_, err := fmt.Sscanf(data[0], "R%d.htm", &num)
			if err == nil {
				reports = append(reports, num)
			}
		}
		data, err = parseTableRow(z, false)
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i] < reports[j]
	})
	return reports
}



func parseMappedReports(docs map[filingDocType]string, docType FilingType) (*financialReport, error) {
	var wg sync.WaitGroup
// 	dataTags := make(map[string]map[string]string)
	fr := newFinancialReport(docType)
	fr.DataTags = make(map[string]map[string]string)
	for t, url := range docs {
		wg.Add(1)
		go func(url string, fr *financialReport, t filingDocType) {
			defer wg.Done()
			page := getPage(url)
			if page != nil {
				page2 := getPage(url)
				dataTagsSub := collectDataTags(page2)
				fr.Lock()
				fr.DataTags[string(t)] = dataTagsSub
				finReportParser(page, fr, t)
				fr.Unlock()
			}
		}(baseURL+url, fr, t)
	}
	wg.Wait()
	return fr, validateFinancialReport(fr)
}
