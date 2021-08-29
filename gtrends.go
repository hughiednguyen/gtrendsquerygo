/************************ Priorities ****************************/
// 1. Get input list through command line
// 2. Call API
// 3.a Sanitize data into format (timestamp, string, scaled interests)
// 3.b Normalization using overlapping ? error handling & special case
// 4. Implement looping / comparison
// 5*. Protobuf the output
/****************************************************************/
// Understanding google trends data
// https://medium.com/@pewresearch/using-google-trends-data-for-research-here-are-6-questions-to-ask-a7097f5fb526

// API Used to query google trends
// https://github.com/groovili/gogtrends

// Normalization technique:
// https://towardsdatascience.com/reconstruct-google-trends-daily-data-for-extended-period-75b6ca1d3420

package main

import (
	"context"
	"fmt"
	"gtrends/pb"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/groovili/gogtrends"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	locUS           = "US"
	catAll          = "all"
	langEn          = "EN"
	MINUTE_INTERVAL = 10
)

type DataRecords map[string]map[int64]int

func main() {
	ctx := context.Background()

	// Create slice from command-line arguments with space-separated
	// list of trend search phrases/words
	// i.e:  ./<program_name> "sentence one" word "sentence 2"
	args := os.Args[1:]

	// Exit immediately if no command line arguments specified
	if len(args) == 0 {
		fmt.Println("No command line arguments specified.")
		return
	}

	// Initialize map of data for all keywords specified
	records := make(map[string]map[int64]int)

	// Temporary record for new query data for one keyword
	newData := make(map[int64]int)

	// Initalize empty map for each record in map
	for _, arg := range args {
		log.Println("Initialized map for:", arg)
		records[string(arg)] = make(map[int64]int)
	}

	// Loop forever (with 10 minutes inbetween each run)
	for {
		// Loop through each keyword to query and process new data
		for _, arg := range args {
			keyword := string(arg)

			// Start code from the example of gogtrends module
			// Get widgets for keyword
			explore, err := gogtrends.Explore(ctx, &gogtrends.ExploreRequest{
				ComparisonItems: []*gogtrends.ComparisonItem{
					{
						Keyword: keyword,
						Geo:     locUS,
						Time:    "now 4-H", // 4 hours timeframe
					},
				},

				Property: "",
			}, langEn)
			handleError(err, "Failed to explore widgets")

			overTime, err := gogtrends.InterestOverTime(ctx, explore[0], langEn)
			handleError(err, "Failed in call interest over time")
			// End code from the example of gogtrends module

			// Reading documentation indicates gogtrends.InterestOverTime()
			// returns slice of pointers to structs data
			for _, i := range overTime {
				// Convert string to Unix Epoch timestamp
				timestamp, _ := strconv.ParseInt(i.Time, 10, 64)

				newData[timestamp] = i.Value[0]
				// DEBUG:
				// fmt.Println("Keyword:", keyword, "Timestamp:", timestamp, "Value", i.Value[0])
			}

			// Special functions to process and normalize value using overlapping
			// also handles API-side error with returning zero instead of actual value

			normalizeData(records[keyword], newData)

			// Reset newData for next query
			for key := range newData {
				delete(newData, key)
			}

			/********************************************************************/
			/************* Output JSON from Protobuf format objects *************/
			/********************************************************************/

			// Obtain unsorted keys from records map
			keys := make([]int64, 0, len(records[keyword]))
			for k := range records[keyword] {
				keys = append(keys, k)
			}

			// Sort the keys
			// Referenced from https://stackoverflow.com/a/48568680
			sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

			// DEBUG outputs current time for the current keyword being queried
			// fmt.Println("Current Time:", time.Now().UTC())
			// fmt.Println("Keyword:", keyword)

			// Construct Protobuf message for each data
			// Marshal into JSON output
			for _, k := range keys {

				// DEBUG: ENABLE THESE TWO TO SEE UTC timestamp and values
				// timestampUTC := time.Unix(k, 0).UTC()
				// fmt.Println("\tTimestamp:", timestampUTC, "Value:", records[keyword][k])

				// Protobuf Message Object
				msg := &pb.Message{
					Keyword:   keyword,
					Timestamp: k,
					Value:     int32(records[keyword][k]),
				}

				// Marshal to JSON format
				jsonBytes, _ := protojson.Marshal(msg)
				fmt.Println(string(jsonBytes))
			}

			fmt.Printf("\n")

			// Allow enough time between query calls to avoid being rate-limited
			time.Sleep(time.Second)
		}
		time.Sleep(MINUTE_INTERVAL * time.Minute)
	}
}

/*******************************************************************
	findScaleValue() calculates a consistent scale value using
	overlapping data points from existing, normalized data
********************************************************************/
func findScaleValue(existingData, newData map[int64]int) float64 {
	// Find a consistent scaling value by calculating scaling for
	// all overlapping, nonzero data points and averaging them
	scaleValueSum := 0.0
	scaleValueAvg := 0.0
	n := 0

	for key, value := range newData {
		_, found := existingData[key]

		if found {
			if existingData[key] != 0 && value != 0 {
				scaleValueSum += float64(existingData[key]) / float64(value)
				n += 1
			}
		}
	}

	// Note: Final scale value can be zero if there are zero interests
	if n != 0 {
		scaleValueAvg = scaleValueSum / float64(n)
	}
	return scaleValueAvg

	/*
		SPECIAL CASE: Let's say there have been no interests for
		over four hours, and then suddenly there are some interests.

		In this case, no data exist (all zeroes) beforehand, so no basis
		to calculate scale from (so n = 0 and scale = 0), and new
		data are nonzero values. Best solution is to do multiple samplings
		from higher timeframes with Google Trends, which is outside
		the scope of the specifications for this assignment where
		results be scaled relative to the first sample.

		Current bandaid solution for this special case is to accept
		new values without scaling and manually perform scaling for any
		subsets that show no interests in a period of 4 hours or more.
	*/
}

/*******************************************************************
	normalizeData() calculates a consistent scale value using
	overlapping datapoints from existing, normalized data
	and reduce existing records down to only normalize, new data
********************************************************************/
func normalizeData(existingData, newData map[int64]int) {
	// If this is the first query (no existing data yet),
	// simply accept new data into existing records
	if len(existingData) == 0 {
		for k, v := range newData {
			existingData[k] = v
		}
		return
	}
	// Else continue on to normalize and process new data

	// Find scale value
	scaleValue := findScaleValue(existingData, newData)

	// Normalize all new data and put into existing data records
	// Note: Existing data points with zero will be overwritten if new data is nonzero
	for key, value := range newData {
		normalizedValue := math.Round(float64(value) * scaleValue)

		_, found := existingData[key]
		if found {
			if existingData[key] == 0 && value != 0 && scaleValue != 0 {
				existingData[key] = int(normalizedValue)
			} else if existingData[key] == 0 && value != 0 && scaleValue == 0 {
				// SPECIAL CASE bandaid solution: Accept new value for
				// later processing, so no scaling needed here
				existingData[key] = value
			}
			// Else: Keep existing value unchanged

		} else { // Brand new time datapoints
			existingData[key] = int(normalizedValue)
		}
	}

	// Prune all existing data points that are not in the new set
	// in order to output only normalized, new data
	for key := range existingData {
		_, found := newData[key]

		if found {
			continue
		} else {
			delete(existingData, key)
		}
	}
}

/*******************************************************************
	handleError() handles errors, implemented by example code
	from gogtrends library
********************************************************************/
func handleError(err error, errMsg string) {
	if err != nil {
		log.Fatal(errors.Wrap(err, errMsg))
	}
}
