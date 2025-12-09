// TODO: Replace with go-gitlab compatible library
package helsegitlab

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/helpers/stringhelper"
)

var httpClient *http.Client
var gitlabToken string

func GetFileContent(projectId int, filePath string, branch string, vaultClient *vaultclient.VaultClient) ([]byte, error) {
	client, token, err := getGitlabClient(vaultClient)
	if err != nil {
		rlog.Error("could not get gitlab client", err)
		return nil, errors.New("could not get gitlab client")
	}

	urlencodeFilePath := url.QueryEscape(filePath)
	urlencodeBranch := url.QueryEscape(branch)

	repourl := fmt.Sprintf("%s%d/repository/files/%s/raw?ref=%s", rorconfig.GetString("HELSEGITLAB_BASE_URL"), projectId, urlencodeFilePath, urlencodeBranch)
	request, err := http.NewRequest(http.MethodGet, repourl, nil)
	if err != nil {
		rlog.Error("could not create helsegitlab request", err)
		return nil, errors.New("could not create helsegitlab request")
	}

	request.Header.Set("PRIVATE-TOKEN", token)

	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("could not get a response from helsegitlab client")
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		rlog.Error("could not read helsegitlab response body", err)
		return nil, errors.New("could not read response body from helsegitlab client")
	}

	statusCode := response.StatusCode
	if statusCode > 299 {
		messages, err := stringhelper.JsonToMap(string(body))
		if err != nil {
			return nil, fmt.Errorf("could not get configuration from ror-api, status code: %d, error: %s", statusCode, string(body))
		}

		return nil, fmt.Errorf("could not get configuration from ror-api, status code: %d, error: %s", statusCode, messages)
	}

	return body, nil
}

func getGitlabClient(vaultClient *vaultclient.VaultClient) (*http.Client, string, error) {
	if httpClient != nil && gitlabToken != "" {
		return httpClient, gitlabToken, nil
	}

	secretPath := "secret/data/v1.0/ror/config/common" // #nosec G101 Jest the path to the token file in the secrets engine
	vaultData, err := vaultClient.GetSecret(secretPath)
	if err != nil {
		return nil, "", errors.New("could not extract gitlab access token from vault")
	}

	commonConfig, ok := vaultData["data"].(map[string]interface{})
	if !ok {
		rlog.Error("", fmt.Errorf("data type assertion failed: %T %#v", vaultData["data"], vaultData["data"]))
		return nil, "", errors.New("could not extract gitlab access token from vault-data")
	}

	tokenValue, ok := commonConfig["helsegitlabToken"].(string)
	if !ok {
		rlog.Error("", fmt.Errorf("helsegitlabToken type assertion failed: %T %#v", commonConfig["helsegitlabToken"], commonConfig["helsegitlabToken"]))
		return nil, "", errors.New("could not extract gitlab access token from vault-data")
	}

	if len(tokenValue) < 1 {
		rlog.Error("could not get gitlab access token from vault", fmt.Errorf("token is empty"))
		return nil, "", errors.New("could not get gitlab access token from vault")
	}

	httpClient = &http.Client{}
	gitlabToken = tokenValue

	return httpClient, gitlabToken, nil
}
