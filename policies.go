/*
Copyright 2023 Derrick J Wippler

Licensed under the MIT License, you may obtain a copy of the License at

https://opensource.org/license/mit/ or in the root of this code repo

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package duh

import (
	"github.com/duh-rpc/duh.go/v2/retry"
)

// RetryableCodes are service response codes that indicate a transient failure.
var RetryableCodes = []int{CodeTooManyRequests, CodeRetryRequest, CodeInternalError}

// RetryableInfraCodes are infrastructure response codes worth retrying.
// NOTE: 404 is intentionally included. An infra 404 means the service is not
// routable (e.g., no backends registered) -- this is transient and worth retrying.
// A service 404 (with Reply body) means "resource not found" and is NOT in
// RetryableCodes, so it won't be retried. The OnCodes/OnInfraCodes split makes
// this distinction safe.
var RetryableInfraCodes = []int{CodeNotFound, 502, 503, 504}

// OnRetryable retries indefinitely on known retryable service codes and
// infrastructure errors. Cancel via context.
var OnRetryable = retry.Policy{
	Interval:     retry.DefaultBackOff,
	OnCodes:      RetryableCodes,
	OnInfraCodes: RetryableInfraCodes,
	Attempts:     0,
}
