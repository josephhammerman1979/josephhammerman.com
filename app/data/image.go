package data

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

const (
	imgStorageBaseDir      = "app/data/imgdata/"
	imageIDDateLayout      = "20060102"
	imageExtension         = ".png"
	imageMetadataExtension = ".metadata"
)

type Image struct {
	ID       string
	Caption  string
	Location string
	Width    int
	Height   int
}

func (i Image) TimeFromID() (*time.Time, error) {
	if len(i.ID) < len(imageIDDateLayout) {
		return nil, fmt.Errorf("unexpected id format: %s", i.ID)
	}
	t, err := time.Parse(imageIDDateLayout, i.ID[:len(imageIDDateLayout)])
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (i *Image) GetImage(id string) (*Image, error) {
	return i.loadMetadata(id)
}

func (i *Image) loadMetadata(id string) (*Image, error) {
	img := &Image{}

	fh, err := os.Open(imgStorageBaseDir + strings.Replace(id, imageExtension, imageMetadataExtension, -1))

	if err != nil {
		fmt.Println(err)
	}

	defer fh.Close()
	metadataRaw, _ := ioutil.ReadAll(fh)

	err = json.Unmarshal(metadataRaw, img)

	if err != nil {
		fmt.Println(err)
	}

	return img, err
}
