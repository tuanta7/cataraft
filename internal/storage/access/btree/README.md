# B+ Tree Implementation

Reference: [programiz.com](https://www.programiz.com/dsa/b-plus-tree)

In a B+tree, only leaf nodes contain value, keys are duplicated in internal nodes to indicate the key range of the subtree.

- Only $n-1$ keys are needed for $n$ intervals, the first key is only used as an extra key for easier visualization.

## 1. Operations

If a B+ tree stores $N$ keys and each node contains at most $s$ keys (where $s$ is a fixed constant), then:

- The height of the tree is $O(\log_s N)$
- At each level, locating the appropriate child pointer costs $O(\log s)$ using binary search

Therefore, the total lookup complexity is $O(\log_s N) * O(\log s)$ which simplifies to $O(\log N)$

For insertion and deletion, after finding the leaf node, updating the leaf node is constant $O(s)$ most of the time.

## 2. Maintaining a B+ Tree

A B+ tree maintains the following invariants

- The root contains a minimum of 1 key and has at least 2 children (or no children at all).
- Each node has at most $m$ children. All nodes except the root must have at least $m/2$ children.
- The tree is height-balanced, all leaves are at the same depth
- Node capacity is bounded, each node contains at most $s$ keys (where $s$ is determined by page size).
- For each node, the keys are stored in increasing order.

### 2.1. Splitting Nodes (Insertion)

If the node exceeds capacity when inserting a key, an overflow occurs.

#### Non-leaf Split

The median separator key is pushed upward. This process may propagate recursively toward the root.

- If the root overflows, a new root is created and the tree height increases by one.
- Balance is preserved automatically.

```
Before: (m = 4)

+-------------------------+
| 10 | 20 | 25 | 30 | 40 |
+-------------------------+

After:

                +------+
                |  25  |
                +------+
                 /    \
                /      \
+---------------+   +---------------+
| 10 | 20       |   | 30 | 40       |
+---------------+   +---------------+
```

#### Leaf Split

The leaf is divided into two nodes

- The left node keeps $\lceil s/2 \rceil$ keys and the right node receives the remaining keys.
- The smallest key in the right node is promoted to the parent as a separator.
- If the parent node is already full, follow non-leaf node splitting rule.

```
Before: (m = 4)

+-------------------------+
| 10 | 20 | 25 | 30 | 40 |
+-------------------------+

After:

             ----+------+----
                 |  30  |
             ----+------+----
                 /     \
                /       \
+---------------+      +---------------+
| 10 | 20 | 25  | ---> | 30 | 40 |     |
+---------------+      +---------------+
```

### 2.2. Shrinking Nodes (Deletion)

Shrinking occurs when a node falls below minimum occupancy (usually $\lceil s/2 \rceil$) after a key deletion. There are two strategies exist:

- **Redistribution** (Preferred): Borrowing from Sibling
- **Merge/Coalescing**: Used if redistribution is impossible

Both may propagate upward

#### Redistribution

Redistribution is possible if a sibling has more than minimum keys.

```
Before: (m = 5)
                +------+
                |  30  |
                +------+
                 /    \
                /      \
+---------------+   +-------------------+
| 10 | 20 | 25* |   | 30 | 40 | 50 | 60 |
+---------------+   +-------------------+

After:

                +------+
                |  40  |
                +------+
                 /    \
                /      \
+---------------+   +---------------+
| 10 | 20 | 30  |   | 40 | 50 | 60  |
+---------------+   +---------------+
```

#### Merge

If both siblings have only the minimum number of keys

```
Before: (m = 4)
                +------+-------
                |  30  | 60 ...
                +------+-------
                 /    \
                /      \
+---------------+   +--------------+
| 10 | 20* |    |   | 30 | 40 |    |
+---------------+   +--------------+

After: Parent separator is removed

                +------+
                |  60  |
                +------+
                 /    \
                /      \
+---------------+   +---------------+
| 10 | 30 | 40  |   | 60 | ...      |
+---------------+   +---------------+
```

#### Root Shrinking

If the root loses all keys

- Height decreases by one.

## 3. Disk Storage

Reference: [Build Your Own Database From Scratch in Go](https://build-your-own.org/database/)

> [!NOTE]
> This B+ Tree implementation is simplified:
>
> - Leaf and internal nodes share the same format. This wastes some space: leaf nodes don’t need pointers and internal nodes don’t need values.
> - Key-value sizes are constrained so entries always fit within a single node.

### 3.1. Node Format

Using the fence-key model, a node with n keys has n child pointers; the first key serves as the lower fence key.

- Indices are numbered from 0 to n−1.
- $key_i$ maps directly to $ptr_i$ (`K0 -> [K0, K1)`, `K1 -> [K1, K2)`, ... `Kn-1 -> [Kn-1, +∞)`).
- There is no pointer to the (-∞, K0) node since the index cannot be negative.
- The keys are stored in increasing order.

```sh
| type | nkeys |  pointers  |  offsets   | key-values | unused |
|  2B  |   2B  | nkeys × 8B | nkeys × 2B |     ...    |        |
```

Each KV pair is prefixed by its size. For internal nodes, the value size is 0.

```sh
| key_size | val_size | key | val |
|    2B    |    2B    | ... | ... |
```

### 3.2. Crash Recovery

#### Copy-on-write

Copy-on-write atomically switches everything to the new version.

#### Double-write
