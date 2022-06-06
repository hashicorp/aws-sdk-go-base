package servicemocks

import (
	"io/ioutil"
	"os"
)

func WriteTempFile(name, content string) (string, func(), error) {
	file, err := ioutil.TempFile("", name)
	if err != nil {
		return "", nil, err
	}

	err = ioutil.WriteFile(file.Name(), []byte(content), 0600)

	return file.Name(), func() { os.Remove(file.Name()) }, err
}
