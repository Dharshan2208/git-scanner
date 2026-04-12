package types

// detected issue structure
type Finding struct {
	File    string
	Line    int
	Type    string
	Match   string
	Commit  string
	Message string

	IntroducedCommit   string 
	RemovedCommit      string 
	ExposureCommits    int    
	ExposureWindow     string 
	StillPresentInHEAD bool   
}
