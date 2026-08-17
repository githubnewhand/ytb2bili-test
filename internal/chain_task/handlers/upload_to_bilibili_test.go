package handlers

import "testing"

func TestSelectOriginalUploadTitleNeverFallsBackToGeneratedTitle(t *testing.T) {
	tests := []struct {
		name           string
		originalTitle  string
		generatedTitle string
		want           string
		wantErr        bool
	}{
		{
			name:           "original title wins",
			originalTitle:  "Original #Title",
			generatedTitle: "AI generated title",
			want:           "Original #Title",
		},
		{
			name:           "missing original blocks upload",
			generatedTitle: "AI generated title",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectOriginalUploadTitle(tt.originalTitle, tt.generatedTitle)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("selectOriginalUploadTitle() = %q, err=%v; want %q, wantErr=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestParseVideoMetadataRequiresOriginalTitle(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
		wantErr bool
	}{
		{name: "valid yt-dlp metadata", payload: `{"title":"Original title","description":"source description"}`, want: "Original title"},
		{name: "missing title", payload: `{"description":"description only"}`, wantErr: true},
		{name: "invalid json", payload: `{`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := parseVideoMetadata([]byte(tt.payload))
			if (err != nil) != tt.wantErr || (!tt.wantErr && metadata.Title != tt.want) {
				t.Fatalf("parseVideoMetadata() = %#v, err=%v; want title %q, wantErr=%v", metadata, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestBilibiliAudienceSettings(t *testing.T) {
	tests := []struct {
		audience  string
		openElec  int
		exclusive int
		level     int
		wantErr   bool
	}{
		{audience: "free"},
		{audience: "charge_30", openElec: 1, exclusive: 1, level: 1},
		{audience: "charge_50", openElec: 1, exclusive: 1, level: 2},
		{audience: "", wantErr: true},
		{audience: "charge_6", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.audience, func(t *testing.T) {
			openElec, exclusive, level, err := bilibiliAudienceSettings(tt.audience)
			if (err != nil) != tt.wantErr {
				t.Fatalf("bilibiliAudienceSettings(%q) err=%v, wantErr=%v", tt.audience, err, tt.wantErr)
			}
			if openElec != tt.openElec || exclusive != tt.exclusive || level != tt.level {
				t.Fatalf(
					"bilibiliAudienceSettings(%q) = (%d,%d,%d), want (%d,%d,%d)",
					tt.audience,
					openElec,
					exclusive,
					level,
					tt.openElec,
					tt.exclusive,
					tt.level,
				)
			}
		})
	}
}
