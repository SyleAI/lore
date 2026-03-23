package ai

// Cluster is a group of ticket IDs with similar embeddings.
type Cluster struct {
	IDs      []string
	Strength float64 // average pairwise cosine similarity within the cluster
}

// FindClusters groups ids by pairwise cosine similarity above threshold.
// The algorithm is greedy: each item joins the first existing cluster whose
// average similarity to existing members is >= threshold, or starts a new one.
// Singletons are excluded from the result.
func FindClusters(ids []string, vecs []Embedding, threshold float64) []Cluster {
	if len(ids) == 0 {
		return nil
	}

	// clusterMembers[i] holds the vector indices in cluster i.
	var clusterMembers [][]int

	for i := range ids {
		bestCluster := -1
		bestSim := -1.0

		for ci, members := range clusterMembers {
			var total float64
			for _, mi := range members {
				total += CosineSimilarity(vecs[i], vecs[mi])
			}
			avg := total / float64(len(members))
			if avg >= threshold && avg > bestSim {
				bestSim = avg
				bestCluster = ci
			}
		}

		if bestCluster >= 0 {
			clusterMembers[bestCluster] = append(clusterMembers[bestCluster], i)
		} else {
			clusterMembers = append(clusterMembers, []int{i})
		}
	}

	var result []Cluster
	for _, members := range clusterMembers {
		if len(members) < 2 {
			continue
		}
		c := Cluster{IDs: make([]string, len(members))}
		for j, mi := range members {
			c.IDs[j] = ids[mi]
		}
		c.Strength = avgPairwiseSimilarity(vecs, members)
		result = append(result, c)
	}
	return result
}

func avgPairwiseSimilarity(vecs []Embedding, members []int) float64 {
	var total float64
	var count int
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			total += CosineSimilarity(vecs[members[i]], vecs[members[j]])
			count++
		}
	}
	if count == 0 {
		return 1.0
	}
	return total / float64(count)
}
