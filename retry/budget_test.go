package retry_test

//func TestMovingRate(t *testing.T) {
//	rate := retry.NewMovingRate(time.Minute, 60)
//
//	now := time.Now()
//	rate.Add(5, now)
//	rate.Add(3, now.Add(1*time.Second))
//	rate.Add(20, now.Add(2*time.Second))
//	rate.Add(20, now.Add(3*time.Second))
//
//	r := rate.Rate(now.Add(20 * time.Second))
//	fmt.Printf("Current rate: %.2f hits per minute\n", r)
//}
