package decoration

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models/Castle"
)

// DecorationCatalogVersion matches Server/Data/EmpireItemsMeta.json (castleItemXMLVersion).
// Regenerate: go run ./Server/cmd/gendecorationwids
const DecorationCatalogVersion = "766.03"

// DecorationWIDs lists every building wodID with type "deco" in EmpireItems/buildings.json (sorted).

var DecorationWIDs = []int{
	6, 54, 55, 59, 60, 64, 65, 66, 67, 69, 70, 71, 72, 95, 96, 97,
	98, 300, 301, 302, 303, 304, 305, 306, 307, 310, 313, 314, 315, 316, 317, 319,
	321, 322, 323, 324, 325, 326, 327, 328, 329, 330, 331, 332, 333, 334, 335, 340,
	341, 343, 344, 345, 346, 347, 348, 349, 356, 357, 358, 359, 360, 361, 362, 363,
	364, 365, 366, 367, 370, 377, 378, 379, 380, 381, 382, 383, 384, 386, 387, 388,
	389, 390, 392, 393, 394, 395, 398, 399, 400, 457, 534, 535, 536, 537, 617, 618,
	619, 632, 633, 637, 638, 639, 641, 759, 762, 836, 1484, 1485, 1488, 1490, 1492, 1493,
	1498, 1501, 1502, 1503, 1504, 1505, 1508, 1509, 1510, 1512, 1513, 1514, 1515, 1516, 1517, 1518,
	1519, 1520, 1521, 1529, 1530, 1532, 1533, 1534, 1535, 1536, 1537, 1538, 1539, 1540, 1541, 1542,
	1543, 1545, 1546, 1547, 1548, 1549, 1550, 1581, 1582, 1583, 1584, 1585, 1586, 1587, 1588, 1589,
	1590, 1591, 1592, 1593, 1594, 1595, 1596, 1597, 1598, 1599, 1642, 1643, 1644, 1645, 1646, 1647,
	1648, 1649, 1651, 1652, 1654, 1655, 1656, 1657, 1658, 1659, 1660, 1661, 1662, 1663, 1664, 1665,
	1666, 1667, 1668, 1669, 1670, 1671, 1672, 1673, 1674, 1675, 1676, 1677, 1678, 1679, 1680, 1681,
	1682, 1683, 1684, 1685, 1686, 1687, 1688, 1689, 1690, 1691, 1692, 1693, 1694, 1695, 1696, 1697,
	1698, 1723, 1724, 1725, 1726, 1727, 1728, 1729, 1730, 1731, 1732, 1733, 1734, 1735, 1736, 1737,
	1738, 1739, 1740, 1745, 1746, 1747, 1752, 1753, 1754, 1755, 1756, 1757, 1758, 1759, 1760, 1761,
	1762, 1768, 1794, 1820, 1866, 1869, 1881, 1882, 1883, 1887, 1888, 1889, 1890, 1891, 1892, 1893,
	1894, 1895, 1896, 1918, 1919, 1922, 1934, 1935, 1936, 1937, 1941, 1974, 1975, 1976, 1977, 1978,
	1979, 1980, 1981, 1982, 1983, 2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008, 2009, 2014, 2015,
	2016, 2017, 2018, 2019, 2040, 2041, 2042, 2043, 2044, 2045, 2046, 2047, 2048, 2049, 2050, 2051,
	2052, 2053, 2054, 2055, 2056, 2057, 2058, 2059, 2060, 2061, 2071, 2072, 2073, 2074, 2075, 2076,
	2077, 2078, 2079, 2080, 2081, 2082, 2083, 2084, 2098, 2099, 2100, 2101, 2102, 2103, 2104, 2105,
	2106, 2107, 2108, 2109, 2110, 2111, 2112, 2113, 2114, 2115, 2116, 2117, 2118, 2119, 2120, 2121,
	2122, 2123, 2124, 2125, 2126, 2127, 2128, 2129, 2130, 2131, 2132, 2133, 2134, 2135, 2136, 2137,
	2138, 2139, 2140, 2141, 2142, 2143, 2144, 2145, 2146, 2147, 2148, 2149, 2150, 2151, 2152, 2153,
	2154, 2155, 2156, 2157, 2158, 2159, 2160, 2161, 2162, 2163, 2164, 2165, 2166, 2167, 2168, 2169,
	2170, 2171, 2178, 2179, 2180, 2181, 2182, 2183, 2184, 2185, 2186, 2187, 2188, 2189, 2190, 2191,
	2192, 2193, 2194, 2195, 2196, 2197, 2198, 2199, 2200, 2201, 2202, 2203, 2204, 2205, 2206, 2207,
	2208, 2209, 2212, 2213, 2214, 2215, 2216, 2217, 2218, 2219, 2220, 2221, 2222, 2223, 2224, 2225,
	2226, 2227, 2228, 2229, 2230, 2231, 2232, 2233, 2234, 2235, 2236, 2237, 2238, 2239, 2240, 2241,
	2242, 2243, 2244, 2245, 2246, 2247, 2248, 2249, 2250, 2251, 2252, 2253, 2254, 2255, 2256, 2257,
	2258, 2259, 2260, 2261, 2262, 2263, 2264, 2265, 2266, 2267, 2268, 2269, 2270, 2271, 2272, 2273,
	2274, 2275, 2276, 2277, 2278, 2279, 2280, 2281, 2282, 2283, 2285, 2286, 2287, 2288, 2289, 2290,
	2291, 2292, 2293, 2294, 2295, 2296, 2297, 2298, 2299, 2300, 2301, 2302, 2303, 2304, 2305, 2306,
	2307, 2308, 2309, 2310, 2311, 2312, 2313, 2314, 2315, 2316, 2317, 2318, 2319, 2320, 2321, 2322,
	2323, 2324, 2325, 2326, 2327, 2328, 2329, 2330, 2331, 2332, 2333, 2334, 2335, 2336, 2337, 2338,
	2339, 2340, 2341, 2342, 2343, 2344, 2345, 2346, 2347, 2348, 2349, 2350, 2351, 2352, 2353, 2354,
	2355, 2356, 2357, 2358, 2359, 2360, 2361, 2362, 2363, 2364, 2365, 2366, 2367, 2368, 2369, 2370,
	2371, 2372, 2373, 2374, 2375, 2376, 2377, 2378, 2379, 2380, 2381, 2382, 2383, 2384, 2385, 2386,
	2387, 2388, 2389, 2390, 2391, 2392, 2393, 2394, 2395, 2396, 2397, 2398, 2399, 2400, 2401, 2402,
	2403, 2404, 2405, 2406, 2407, 2408, 2409, 2410, 2411, 2412, 2413, 2414, 2415, 2416, 2417, 2418,
	2419, 2420, 2421, 2422, 2423, 2424, 2425, 2426, 2427, 2428, 2429, 2430, 2431, 2432, 2433, 2434,
	2435, 2436, 2437, 2438, 2439, 2440, 2441, 2442, 2443, 2444, 2445, 2446, 2447, 2448, 2449, 2450,
	2451, 2452, 2453, 2454, 2455, 2456, 2457, 2458, 2459, 2460, 2461, 2462, 2463, 2464, 2465, 2466,
	2467, 2468, 2469, 2470, 2471, 2472, 2473, 2474, 2475, 2476, 2477, 2478, 2479, 2480, 2481, 2482,
	2483, 2484, 2485, 2486, 2487, 2488, 2489, 2490, 2491, 2492, 2493, 2494, 2495, 2496, 2497, 2498,
	2499, 2500, 2501, 2502, 2503, 2504, 2505, 2506, 2507, 2508, 2509, 2510, 2511, 2512, 2513, 2514,
	2515, 2516, 2517, 2518, 2519, 2520, 2521, 2522, 2523, 2524, 2525, 2526, 2527, 2528, 2529, 2530,
	2531, 2532, 2533, 2534, 2535, 2536, 2537, 2538, 2539, 2540, 2541, 2550, 2551, 2552, 2553, 2554,
	2555, 2556, 2557, 2558, 2559, 2560, 2561, 2562, 2563, 2564, 2565, 2566, 2567, 2568, 2569, 2570,
	2571, 2572, 2573, 2574, 2575, 2576, 2577, 2578, 2579, 2580, 2581, 2582, 2583, 2584, 2585, 2586,
	2587, 2588, 2589, 2590, 2591, 2592, 2593, 2594, 2595, 2596, 2597, 2598, 2599, 2600, 2601, 2602,
	2603, 2604, 2605, 2606, 2607, 2608, 2609, 2610, 2611, 2612, 2613, 2614, 2615, 2616, 2617, 2618,
	2619, 2620, 2621, 2622, 2623, 2624, 2625, 2626, 2627, 2628, 2629, 2630, 2631, 2632, 2633, 2634,
	2635, 2636, 2637, 2638, 2639, 2640, 2641, 2642, 2643, 2644, 2645, 2646, 2647, 2648, 2649, 2650,
	2651, 2652, 2653, 2654, 2655, 2656, 2657, 2658, 2659, 2660, 2661, 2662, 2663, 2664, 2665, 2666,
	2667, 2668, 2669, 2670, 2671, 2672, 2673, 2674, 2675, 2676, 2677, 2678, 2679, 2680, 2681, 2682,
	2683, 2684, 2685, 2686, 2687, 2688, 2689, 2690, 2691, 2692, 2693, 2694, 2695, 2696, 2697, 2698,
	2699, 2700, 2701, 2702, 2703, 2704, 2705, 2706, 2707, 2708, 2709, 2710, 2711, 2712, 2713, 2714,
	2715, 2716, 2717, 2718, 2719, 2720, 2721, 2722, 2723, 2724, 2725, 2726, 2727, 2728, 2729, 2730,
	2731, 2732, 2733, 2734, 2735, 2736, 2737, 2738, 2739, 2740, 2743, 2744, 2745, 2746, 2747, 2748,
	2749, 2750, 2751, 2752, 2753, 2754, 2755, 2756, 2757, 2758, 2759, 2760, 2761, 2762, 2763, 2764,
	2765, 2766, 2767, 2768, 2769, 2770, 2771, 2772, 2773, 2774, 2775, 2776, 2777, 2778, 2779, 2780,
	2781, 2782, 2783, 2784, 2785, 2786, 2787, 2788, 2789, 2790, 2791, 2792, 2793, 2794, 2795, 2796,
	2797, 2798, 2799, 2800, 2801, 2802, 2803, 2805, 2806, 2807, 2808, 2809, 2810, 2811, 2812, 2813,
	2814, 2815, 2816, 2817, 2818, 2819, 2820, 2821, 2822, 2823, 2824, 2825, 2826, 2827, 2828, 2829,
	2830, 2831, 2832, 2833, 2834, 2835, 2836, 2837, 2838, 2839, 2840, 2841, 2842, 2843, 2844, 2845,
	2846, 2847, 2848, 2849, 2850, 2851, 2852, 2853, 2854, 2855, 2856, 2857, 2858, 2859, 2860, 2861,
	2862, 2863, 2864, 2865, 2866, 2867, 2868, 2869, 2870, 2871, 2872, 2873, 2874, 2875, 2876, 2877,
	2878, 2879, 2880, 2896, 2897, 2911, 2912, 2913, 2914, 2915, 2916, 2917, 2918, 2919, 2920, 2921,
	2922, 2923, 2924, 2925, 2926, 2927, 2928, 2929, 2930, 2931, 2932, 2933, 2934, 2935, 2936, 2937,
	2938, 2939, 2940, 2941, 2942, 2943, 2944, 2945, 2946, 2947, 2948, 2949, 2950, 2951, 2952, 2953,
	2954, 2955, 2956, 2957, 2958, 2959, 2960, 2961, 2962, 2963, 2964, 2965, 2966, 2967, 2968, 2969,
	2970, 2971, 2973, 2974, 2975, 2976, 2977, 2978, 2979, 2980, 2981, 2982, 2983, 2984, 2985, 2986,
	2994, 2995, 2996, 2997, 3000, 3001, 3002, 3003, 3004, 3005, 3006, 3007, 3008, 3009, 3010, 3011,
	3012, 3013, 3014, 3015, 3016, 3017, 3018, 3019, 3108, 3109, 3110, 3142, 3143, 3144, 3145, 3146,
	3147, 3148, 3149, 3150, 3151, 3152, 3153, 3154, 3155, 3156, 3157, 3158, 3159, 3160, 3161, 3162,
	3163, 3164, 3165, 3166, 3167, 3168, 3169, 3170, 3171, 3172, 3173, 3174, 3176, 3177, 3178, 3179,
	3180, 3184, 3185, 3186, 3187, 3188, 3189, 3190, 3191, 3192, 3193, 3194, 3195, 3197, 3198, 3212,
	3213, 3214, 3215, 3216, 3220, 3221, 3222, 3223, 3224, 3225, 3226, 3227, 3228, 3229, 3230, 3231,
	3232, 3233, 3234, 3235, 3236, 3237, 3238, 3239, 3240, 3241, 3242, 3243, 3244, 3245, 3246, 3247,
	3248, 3249, 4251, 4264, 4275, 4276, 4277, 4278, 4279, 4280, 4281, 4282, 4283, 4284, 4285, 4286,
	4287, 4288, 4289, 4290, 4291, 4292, 4293, 4294, 4295, 4296, 4297, 4298, 4299, 4300, 4301, 4302,
	4303, 4304, 4305, 4306, 4307, 4308, 4309, 4310, 4313, 4314, 4315, 4316, 4317, 4318, 4319, 4320,
	4321, 4322, 4323, 4324, 4325, 4326, 4327, 4328, 4329, 4330, 4331, 4332, 4333, 4334, 4335, 4336,
	4337, 4338, 4339, 4340, 4341, 4342, 4343, 4344, 4345, 4346, 4347, 4348, 4349, 4350, 4351, 4352,
	4353, 4354, 4355, 4356, 4357, 4358, 4359, 4360, 4361, 4362, 4363, 4364, 4365, 4366, 4367, 4368,
	4369, 4370, 4371, 4372, 4373, 4374, 4375, 4376, 4377, 4378, 4379, 4380, 4381, 4382, 4383, 4384,
	4385, 4386, 4387, 4388, 4389, 4390, 4397, 4398, 4399, 4400, 4401, 4402, 4403, 4404, 4405, 4406,
	4407, 4408, 4409, 9000, 9001, 9002, 9003, 9004, 9005, 9006, 9007, 9008, 9009, 9010, 9011, 9012,
	9013, 9014, 9015, 9016, 9017, 9018, 9019, 9020, 9021, 9022, 9023, 9024, 9025, 9026, 9027, 9028,
	9029, 9030, 9031, 9032, 9033, 9034, 9035, 9036, 9037, 9038, 9039, 9040, 9041, 9042, 9043, 9044,
	9045, 9046, 9047, 9048, 9049, 9050, 9051, 9052, 9053, 10000, 10001, 10002, 10003, 10004, 10005, 10006,
	10007, 10008, 10009, 10010, 10011, 10012, 10013, 10014, 10015, 10016, 10017, 10018, 10019, 10020, 10021, 10022,
	10023, 10024, 10025, 10026, 10027, 10028, 10029, 10030, 10031, 10032, 10033, 10034, 10035, 10036, 10037, 10038,
	10039, 10040, 10041, 10042, 10043, 10044, 10045, 10046, 10047, 10048, 10049, 10050, 10051, 10052, 10053, 10054,
	10055, 10056, 29998, 29999, 30000, 30001, 30002,
}

var (
	decorationWIDOnce   sync.Once
	decorationWIDLookup map[int]struct{}
)

func decorationWIDInit() {
	decorationWIDOnce.Do(func() {
		decorationWIDLookup = make(map[int]struct{}, len(DecorationWIDs))
		for _, id := range DecorationWIDs {
			decorationWIDLookup[id] = struct{}{}
		}
	})
}

var (
	buildingsJSONOnce    sync.Once
	decoNameByWID        map[int]string // type "deco" only (legacy DecorationDisplayName)
	allBuildingName      map[int]string // every wodID → name from buildings.json (lazy-loaded once)
	donationEventDecoWID map[int]struct{}
	buildingsJSONErr     error
)

func loadBuildingsJSONNameMaps() {
	buildingsJSONOnce.Do(func() {
		raw, err := data.ReadEmpireItemsSection("buildings")
		if err != nil {
			buildingsJSONErr = err
			return
		}
		var rows []struct {
			WodID    int    `json:"wodID"`
			Name     string `json:"name"`
			Type     string `json:"type"`
			Comment1 string `json:"comment1"`
		}
		if err := json.Unmarshal(raw, &rows); err != nil {
			buildingsJSONErr = err
			return
		}
		decoNameByWID = make(map[int]string)
		allBuildingName = make(map[int]string, len(rows))
		donationEventDecoWID = make(map[int]struct{})
		for _, r := range rows {
			allBuildingName[r.WodID] = r.Name
			if strings.EqualFold(r.Type, "deco") {
				decoNameByWID[r.WodID] = r.Name
				nm := strings.ToLower(strings.TrimSpace(r.Name))
				c1 := strings.ToLower(strings.TrimSpace(r.Comment1))
				if strings.Contains(c1, "donationevent") || strings.Contains(nm, "donation event") {
					donationEventDecoWID[r.WodID] = struct{}{}
				}
			}
		}
	})
}

func loadDecoNamesFromBuildings() { loadBuildingsJSONNameMaps() }

// IsKnownDecorationWID reports whether wid is listed as a decoration in EmpireItems buildings.json.
func IsKnownDecorationWID(wid int) bool {
	decorationWIDInit()
	_, ok := decorationWIDLookup[wid]
	return ok
}

// DecorationDisplayName returns the display name from buildings.json for a decoration WID, if present.
func DecorationDisplayName(wid int) (string, bool) {
	loadBuildingsJSONNameMaps()
	if buildingsJSONErr != nil {
		return "", false
	}
	s, ok := decoNameByWID[wid]
	s = strings.TrimSpace(s)
	return s, ok && s != ""
}

// EmpireBuildingDisplayName returns the localized name for any wodID in EmpireItems buildings.json.
func EmpireBuildingDisplayName(wid int) (string, bool) {
	loadBuildingsJSONNameMaps()
	if buildingsJSONErr != nil {
		return "", false
	}
	s, ok := allBuildingName[wid]
	s = strings.TrimSpace(s)
	return s, ok && s != ""
}

type decorationCount struct {
	wid   int
	count int
	name  string
}

// isDonationEventDecorationWID is true for EmpireItems deco rows tied to donation events
// (comment1 DonationEvent* / name "Donation Event …") — not player cosmetics; omit from focus summary.
func isDonationEventDecorationWID(wid int) bool {
	loadBuildingsJSONNameMaps()
	if buildingsJSONErr != nil {
		return false
	}
	_, ok := donationEventDecoWID[wid]
	return ok
}

// DecorationSummaryLinesForCastle returns sorted lines like "1x Rose Bush" / "3x Supplies" for rows whose WID is a
// known EmpireItems decoration (type "deco" in buildings.json), excluding donation-event decos.
// IsDecorationPickupCandidateWID stays broader for preset/SOB flows (e.g. generic tower WIDs).
// Names prefer WodDisplayNames.json (regenerate: go run ./Server/cmd/genwoddisplaynames),
// then raw buildings.json (see resolvedDisplayNameForWodID).
func DecorationSummaryLinesForCastle(c *castle.PlayerCastleInfo) []string {
	if c == nil {
		return nil
	}
	countByWID := make(map[int]int)
	for _, b := range c.BGRows {
		if IsKnownDecorationWID(b.BuildingID) && !isDonationEventDecorationWID(b.BuildingID) {
			countByWID[b.BuildingID]++
		}
	}
	for _, b := range c.BDRows {
		if IsKnownDecorationWID(b.BuildingID) && !isDonationEventDecorationWID(b.BuildingID) {
			countByWID[b.BuildingID]++
		}
	}
	if len(countByWID) == 0 {
		return nil
	}
	pairs := make([]decorationCount, 0, len(countByWID))
	for wid, n := range countByWID {
		name := resolvedDisplayNameForWodID(wid)
		pairs = append(pairs, decorationCount{wid: wid, count: n, name: name})
	}
	sort.Slice(pairs, func(i, j int) bool {
		ci := strings.ToLower(pairs[i].name)
		cj := strings.ToLower(pairs[j].name)
		if ci != cj {
			return ci < cj
		}
		return pairs[i].wid < pairs[j].wid
	})
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = fmt.Sprintf("%dx %s", p.count, p.name)
	}
	return out
}

// resolvedDisplayNameForWodID prefers WodDisplayNames.json (General's Camp–style lang + items),
// then the raw buildings.json name field.
func resolvedDisplayNameForWodID(wid int) string {
	if s, ok := data.WodDisplayName(wid); ok {
		return s
	}
	if s, ok := EmpireBuildingDisplayName(wid); ok {
		return s
	}
	return data.FormatUnknownWod(wid)
}

// IsEssentialCastleStructureByName matches core / production / defense buildings that must not be
// bulk-removed as "decorations". Cosmetic items typically do not match these substrings.
func IsEssentialCastleStructureByName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || n == "unknown" {
		return true
	}
	essentials := []string{
		"barracks", "tower", "wall", "gate", "moat", "keep", "hospital", "stables",
		"farm", "woodcutter", "quarry", "mine", "mill", "market", "storehouse", "dwelling",
		"academy", "temple", "winery", "bakery", "apiary", "armory", "drill ground",
		"training grounds", "headquarters", "field kitchen", "ballista", "flame tower",
		"wood stock", "stone stock", "vault", "cartographer", "town house", "townhouse",
		"relic woodcutter", "relic quarry", "relic mine", "relic farmstead", "relic mill",
		"construction crane", "watchtower", "sawmill", "brickworks", "foundry", "glassworks",
		"estate", "granary", "workshop", "forge", "furnace", "kiln", "smelter",
	}
	for _, e := range essentials {
		if strings.Contains(n, e) {
			return true
		}
	}
	return false
}

// IsDecorationPickupCandidateWID is true when wid is in the decoration WID list, or when castle
// building metadata suggests a non-essential cosmetic (fallback for newer WIDs).
func IsDecorationPickupCandidateWID(wid int) bool {
	if IsKnownDecorationWID(wid) {
		return true
	}
	info := castle.GetBuildingInfo(wid)
	if info.Name == "Unknown" {
		return false
	}
	return !IsEssentialCastleStructureByName(info.Name)
}

// DecorationSOBBlockedWID is true for building type IDs the server rejects for EmpireEx sob pickup (e.g. status 61).
// Do not use IsDecorationPickupCandidateWID here: many live decorations share generic WIDs (e.g. 201 / "Tower") that
// must still be cleared off preset tiles.
func DecorationSOBBlockedWID(wid int) bool {
	switch wid {
	case 756, 1422, 2027: // construction yard, hall of legends, mead distillery — observed SOB 61
		return true
	default:
		return false
	}
}
