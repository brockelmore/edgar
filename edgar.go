package edgar

import (
	"io"
)

type FilingType string

var (
	//Filing types
	FilingType10Q FilingType = "10-Q"
	FilingType10K FilingType = "10-K"
)

// Date defines an interface for filing date
// This is mainly to validate the date being passed into the package.
type Date interface {
	Day() int
	Month() int
	Year() int
	String() string
}

// Filing interface for fetching financial data
type Filing interface {
	Ticker() string
	FiledOn() Date
	Type() (FilingType, error)
	ShareCount() (int64, error)
	Revenue() (int64, error)
	CostOfRevenue() (int64, error)
	GrossMargin() (int64, error)
	OperatingIncome() (int64, error)
	OperatingExpense() (int64, error)
	NetIncome() (int64, error)
	TotalEquity() (int64, error)
	ShortTermDebt() (int64, error)
	LongTermDebt() (int64, error)
	CurrentLiabilities() (int64, error)
	DeferredRevenue() (int64, error)
	RetainedEarnings() (int64, error)
	OperatingCashFlow() (int64, error)
	CapitalExpenditure() (int64, error)
}

// Company interface used to get information and filing about a company
type CompanyFolder interface {

	// GetTicker gets the ticker of this company
	Ticker() string

	//AvailableFilings gets the list of dates of available filings
	AvailableFilings(FilingType) []Date

	// Filing gets a filing given a filing type and date of filing.
	Filing(FilingType, Date) (Filing, error)

	// SaveFolder persists the data from the company folder into a writer
	// provided by the user. This stored info can be presented back to
	// the fetcher (using CreateFolder API in fetcher) to recreate the
	// company folder with already parsed data
	SaveFolder(w io.Writer) error

	// String is a dump routine to view the contents of the folder
	String() string
}

// FilingFetcher fetches the filing requested
type FilingFetcher interface {

	// GetFilings creates a folder for the company with a list of
	// available filings. No financial data is pulled and the user
	// of the interface can selectively pull financial data into the
	// folder using the CompanyFolder interface
	CompanyFolder(string, ...FilingType) (CompanyFolder, error)

	// CreateFolder creates a company folder using a Reader
	// User can provoder a store of edgar data previous stored
	// by this package (using the Store function of the Company Folder)
	// This function is used to avoid reparsing edgar data and reusing
	// already parsed and stored information.
	CreateFolder(io.Reader, ...FilingType) (CompanyFolder, error)
}