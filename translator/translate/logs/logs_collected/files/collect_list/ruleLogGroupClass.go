// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package collect_list

import (
	"github.com/aws/amazon-cloudwatch-agent/tool/util"
	"github.com/aws/amazon-cloudwatch-agent/translator"
)

const LogGroupClassSectionKey = "log_group_class"

type LogGroupClass struct {
}

func (f *LogGroupClass) ApplyRule(input interface{}) (returnKey string, returnVal interface{}) {
	_, returnVal = translator.DefaultLogGroupClassCase(LogGroupClassSectionKey, util.StandardLogGroupClass, input)
	returnKey = LogGroupClassSectionKey
	return
}

func init() {
	l := new(LogGroupClass)
	r := []Rule{l}
	RegisterRule(LogGroupClassSectionKey, r)
}
