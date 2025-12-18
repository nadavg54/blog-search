package content

import "testing"

// TestFindTranscriptURL_TiDBPDF verifies that we can locate the PDF transcript link
// from the TiDB episode HTML snippet.
func TestFindTranscriptURL_TiDBPDF(t *testing.T) {
	htmlSnippet := `
<p>Transcript provided by We Edit Podcasts. Software Engineering Daily listeners can go to
<a href="https://weeditpodcasts.com/sed">weeditpodcasts.com/sed</a>
to get 20% off the first two months of audio editing and transcription services. Thanks to We Edit Podcasts for partnering with SE Daily.
<a href="http://softwareengineeringdaily.com/wp-content/uploads/2019/01/SED754-TiDB.pdf">Please click here to view this show’s transcript.</a>
</p>`

	got, err := FindTranscriptURL(htmlSnippet)
	if err != nil {
		t.Fatalf("FindTranscriptURL returned error: %v", err)
	}

	want := "http://softwareengineeringdaily.com/wp-content/uploads/2019/01/SED754-TiDB.pdf"
	if got != want {
		t.Fatalf("FindTranscriptURL = %q, want %q", got, want)
	}
}

// TestFindTranscriptURL_ErikTXT verifies that we can locate the TXT transcript link
// from the Erik Seidel episode HTML snippet.
func TestFindTranscriptURL_ErikTXT(t *testing.T) {
	htmlSnippet := `
<p><a href="http://softwareengineeringdaily.com/wp-content/uploads/2025/10/SED1867-Cloudflare.txt">Please click here to see the transcript of this episode.</a></p>`

	got, err := FindTranscriptURL(htmlSnippet)
	if err != nil {
		t.Fatalf("FindTranscriptURL returned error: %v", err)
	}

	want := "http://softwareengineeringdaily.com/wp-content/uploads/2025/10/SED1867-Cloudflare.txt"
	if got != want {
		t.Fatalf("FindTranscriptURL = %q, want %q", got, want)
	}
}
