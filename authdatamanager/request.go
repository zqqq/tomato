package authdatamanager

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/lfq7413/tomato/types"
)

func request(path string, headers map[string]string) (types.M, error) {
	request, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		request.Header.Set(k, v)
	}

	client := http.DefaultClient
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var result types.M
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
