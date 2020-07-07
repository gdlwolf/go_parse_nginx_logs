package gtool

import log "github.com/sirupsen/logrus"

func SimpleCheckError(e error) {
	if e != nil {
		log.Error(e)
	}
}
