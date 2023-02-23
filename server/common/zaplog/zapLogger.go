// ///////////////////////////////////////
// 2022 SHAI Lab all rights reserved
// ///////////////////////////////////////
package zaplog

import (
	"fmt"
	"os"

	"go.uber.org/zap"
)

var ZapLogger *zap.Logger

func init() {
	var err error
	ZapLogger, err = zap.NewDevelopment()
	if err != nil {
		fmt.Printf("ZapLogger init failed. \n")
		os.Exit(1)
	}
	defer ZapLogger.Sync()
}
