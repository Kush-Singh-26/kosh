---
title: "5. Maths4ML: Matrices"
date: "2025-12-25"
description: "Matrices are Machines, Not Just Grids"
tags: ["Maths for ML"]
pinned: false
---

## Matrices are Space Warpers

A matrix is not just a spreadsheet or a container for data. It is a function or a machine. Ab equation like $y = Ax$ (in a Neural Network or linear regression, etc.), the matrix $A$ is a function or an agent that grabs the data vector ($x$) and physically moves it, warps it and transforms it into a new position ($y$).

Matrices can stretch space and collapse dimensions.

### Matrix transforming the Basis Vectors

Let the basis vectors be denoted by :
- $\hat i$ : $\color{green}{[1,0]}$ - A Green arrow pointing right.
- $\hat j$ : $\color{red}{[0,1]}$ - A Red arrow pointing up.

> The **columns** of a matrix tells exactly where these 2 arrows will land after transformation.

$$ A = \begin{bmatrix} \color{green}{3} & \color{red}{-1} \\ \color{green}{0} & \color{red}{2} \end{bmatrix} $$

- **Column 1** $ \color{green}{(3,0)} $ : The $\hat i$ basis vector now lives here from $(1,0)$.
- **Column 2** $ \color{red}{(-1,2)} $ : The $\hat j$ basis vector now lives here from $(0,1)$.

- Every other point on the grid will follow the new grid lines (basis) formed by these 2 arrows.

So, for example a point $[2,1]$ according to the old system would have meant to go 2 units of $\hat i$ and 1 unit of $\hat j$, but now since these basis vectors point in different direction, the same vector $[2,1]$ will point in a completely different direction.

## Fundamental Matrices

### 1. Scaling

Doubling the length of the green & red arrow will cause the matrix to *zoom in* on the data. For example :

$$ \begin{bmatrix} 2 & 0 \\ 0 & 2 \end{bmatrix} $$

### 2. Rotation

Arrows stay the same in length but pivot $90\degree$, the matrix spins the entire world. The grid will remain square but *tilted* :

$$ \begin{bmatrix} 0 & -1 \\ 1 & 0 \end{bmatrix} $$

### 3. Shearing

If the bottom of a square if fixed and the top is pushed sideways. The square will turn into a *parallelogram*.

$$ \begin{bmatrix}1 & 1 \\ 0 & 1 \end{bmatrix} $$

---

Below image shows the exact matrices transformations as covered above.

![Standard Matrix transformations (Scaling, Rotation & SHearing)](/static/images/M4ML5.png)

## Matrix Vector Multiplication

$$ \begin{bmatrix}a & b \\ c & d \end{bmatrix} \begin{bmatrix} x \\ y \end{bmatrix} = \begin{bmatrix} ax+by \\ cx+dy \end{bmatrix} $$

This is **Row-by-Column** multiplication. It is computationally correct but it doesn't offer any intuition. This same thing can be represented as :

$$ x \cdot \begin{bmatrix} \color{green}{a} \\ \color{green}{c}\end{bmatrix} + y \cdot \begin{bmatrix} \color{red}b \\ \color{red}d\end{bmatrix} $$

This is literally saying :

> Take $x$ steps along the transformed <span class ="green">Green Arrow</span> (Column 1) and then take $y$ steps along the transformed <span class ="red">Red Arrow</span> (Column 2).

## Matrix-Matrix Multiplication

A matrix-matrix multiplication like $C = AB$ using standard row-by-column method is a mess of numbers.

A better way is to **look at matrix $B$ as a collection of columns (vectors)**.

$$ A \times B = A \times [\mathbf{v_1} | \mathbf{v_2}] $$   

So now instead of one big operation it is just doing matrix-vector multiplication twice, once for each of the column of $B$.

- Column 1 of result : $A$ acts on the first column of $B$.
- Column 2 of result : $A$ acts on the second column of $B$.
 
$$\underbrace{\begin{bmatrix} \color{green}{a} & \color{red}{b} \\ \color{green}{c} & \color{red}{d} \end{bmatrix}}_{A}
\underbrace{\begin{bmatrix} x_1 & x_2 \\ y_1 & y_2 \end{bmatrix}}_{B} = C$$

- Column 1 of matrix $C$ is the result of passing column 1 of $B$ through machine (matrix) $A$ :

$$ \text{Col } 1 = x_1 \begin{bmatrix}\ \color{green}{a} \\ \color{green}{c} \end{bmatrix} + y_1 \begin{bmatrix}\ \color{red}{b} \\ \color{red}{d} \end{bmatrix} $$

- Column 2 of matrix $C$ is the result of passing column 2 of $B$ through machine (matrix) $A$ :

$$ \text{Col } 2 = x_2 \begin{bmatrix}\ \color{green}{a} \\ \color{green}{c} \end{bmatrix} + y_2 \begin{bmatrix}\ \color{red}{b} \\ \color{red}{d} \end{bmatrix} $$

---

So, finally Matrix $C$ is just these 2 results pasted side-by-side :

$$C = \left[
    \left( x_1 \begin{bmatrix} a \\ c \end{bmatrix} + y_1 \begin{bmatrix} b \\ d \end{bmatrix} \right) 
   \bigg|
    \left( x_2 \begin{bmatrix} a \\ c \end{bmatrix} + y_2 \begin{bmatrix} b \\ d \end{bmatrix} \right) 
\right]$$

Thus, 
> The output columns of $C$ are literally just **weighted sums of the columns** of $A$. The resulting shape must live inside the space defined by $A$'s columns.

## Matrix Multiplication is <u>Function Composition</u>

Matrix multiplication is simply chaining multiple machines.

$$ y = A(B(x)) $$

The vector $x$ is the raw material.
- Machine $B$ is the first machine which transforms it.
- Machine $A$ is the second machine which grabs the result of Machine $B$ and transforms it again.

### Why MatMul is not commutative

Suppose 2 matrices $S$ & $R$ which stretch the $x$-axis by 2 and rotate everything by $90\degree$ respectively.

$$ S =  \begin{bmatrix} 2 & 0 \\ 0 & 1\end{bmatrix} , \, R \begin{bmatrix} 0 & 1 \\ -1 & 0 \end{bmatrix} $$

#### Scenario 1 : Stretch then rotate

- $y = RSx$
- Stretches left-right.
- Rotates so that left becomes bottom & right becomes top.

#### Scenario 2 : Rotate then stretch

- $y = SRx$
- Rotates so that left becomes bottom & right becomes top.
- Stretches the original top-bottom (which are now left-right).

> Thus, even though the same operations are applied, the order changes everything. Thus, $AB \ne BA$.

## Invertible Matrices

Chaining matrices to get back to from where we started.

- Matrix $A$ is a *Shear Right* matrix.

$$ A = \begin{bmatrix} 1 & 1 \\ 0 & 1\end{bmatrix} $$

- Matrix $B$ is a *Shear Left* matrix.

$$ B = \begin{bmatrix} 1 & -1 \\ 0 & 1\end{bmatrix} $$

Therefore, $y = BAx$ will slant a square right and then push it back to the original shape.

Thus, $B = A^{-1}$

$$ B \times A = \begin{bmatrix} 1&0 \\0&1\end{bmatrix}  = \text{I}$$

$I$ is the identity matrix that does nothing.

## Transpose 

Mechanical definition is to swap the rows & columns.

$$ A = \begin{bmatrix} 1&2&3 \\ 4&5&6 \end{bmatrix} \quad \text{, }A^{\top}\begin{bmatrix} 1&4 \\ 2&5 \\ 3&6 \end{bmatrix}$$

### Co-variance

$X^{\top}X$ is the similarity map of data $X$. Let there be a dataset of 3 students consisting of their study time & score.

$$X = \begin{bmatrix}\text{Student 1} \\\text{Student 2} \\\text{Student 3}\end{bmatrix}=\begin{bmatrix}\color{blue}{-1} & \color{red}{-2} \\\color{blue}{0} & \color{red}{0} \\\color{blue}{1} & \color{red}{2}\end{bmatrix}$$

- Column 1 : <span class = "blue">Blue vector</span> is the *study vector*. $s = [-1,0,1]$
- Column 2 : <span class = "red">Red vector</span> is the *score vector*. $g = [-2,0,2]$

$$X^{\top} = \begin{bmatrix}
\color{blue}{-1} & \color{blue}{0} & \color{blue}{1} \\
\color{red}{-2} & \color{red}{0} & \color{red}{2}
\end{bmatrix}$$

Now Row 1 is the study vector & Row 2 is the score vector.

Thus, $X^{\top}X $ will become :

$$ \begin{bmatrix} \text{Study} \\ \text{Score} \end{bmatrix} \cdot \begin{bmatrix} \text{Study} & \text{Score} \end{bmatrix} = \begin{bmatrix} 2&4 \\ 4&8 \end{bmatrix} $$

- Cell (1,1) : Variance of the Study vector.
- Cell (2,2) : Variance of the Score vector.
- Cell (1,2) & (2,1) : Covariance of the Score & Study vectors.

$$\begin{bmatrix}\text{Variance(Study)} & \text{Covariance(Study, Score)} \\\text{Covariance(Score, Study)} & \text{Variance(Score)}\end{bmatrix}$$

- Diagonals: How spread out is this feature? (Variance)
- Off-Diagonals: How much does Feature A look like Feature B? (Covariance/Similarity)

## Symmetric Matrix

Matrix is equal to its own transpose. $A_{ij} = A_{ji}$

$$ A = A^{\top} $$

Let :

$$ A = \begin{bmatrix}1&2 \\ 0&1 \end{bmatrix} $$

This is an **Asymmetric Matrix**. If the input is a circle, this matrix will grab the top & slide it sideways. The result will be a oval but it will be smeared.

$$ S = \begin{bmatrix}2&1 \\ 1&2 \end{bmatrix} $$

This is a symmetric matrix. It will also stretch a circle but the resultant will be an ellipse with its major & minor axis **perpendicular to each other**.

>- **Asymmetric Matrix** : Might shear space, twist it, and squash it at weird angles.
>- **Symmetric Matrix** : It creates a shape where the axes of stretching are perpendicular.

## Trace 

The trace ($\text{Tr}(A)$) of the matrix $A$ is sum of its diagonal elements.

In a matrix $A = \begin{bmatrix} a & b \\ c & d\end{bmatrix} $ :

- Off diagonal elements :
    - $c$ : tells how much $\hat i$ points Up into the y-axis.
    - $b$ : tells how much $\hat j$ points Right into the x-axis.
> They describe how much $x$ becomes $y$ and $y$ becomes $x$.

- Diagonal elements :
    - $a$ : tells how much $\hat i$ stretches while staying along the x-axis.
    - $d$ : tells how much $\hat j$ stretches while staying along the y-axis.
> They describe the direct stretching.

So $\text{Tr}(A) = a+d$ tells how much the matrix pushing outward along the original grid lines.

> The Trace ignores the mixing. It only asks: "On average, is the machine stretching things out or shrinking them in?"

- **Trace > 0** : Matrix is generally expanding the space.
- **Trace < 0** : Matrix is generally collapsing the space.
- **Trace = 0** : The expansion in one direction is perfectly cancelled by contraction in the other.

## Range (Column Space)

> Range of a matrix is the Span of its columns.

- **Span** of a set of vectors is the set of all the vectors that can be formed by scaling & adding those vectors.

Thus, Column Space (*Range*) is the set of vectors that can be get by taking all possible linear combinations of its column vectors.

$$\text{Range}(A) = \text{Span}(\text{Column 1}, \text{Column 2}, ...)$$

---

### Null Space

The null space (or kernel) of a matrix $A$ is the set of all vectors $\mathbf{x}$ that satisfy the equation $A\mathbf{x}=\mathbf{0}$ (the zero vector).

---

## Rank

It is a single number which measures the **dimension** of the space. It tells the number of actual, non-redundant columns in a matrix.

$$A = \begin{bmatrix}\mathbf{1} & \mathbf{0} & \mathbf{1} \\\mathbf{0} & \mathbf{1} & \mathbf{1}\end{bmatrix}$$

- Column 3 = Column 1 + Column 2.
    - Thus, there are only 2 dimensions as the third column is just a diagonal lying in the plane defined by the first 2 columns.
    - Thus, **Rank = 2**.

The concept of *dimensionality reduction* is based on this fact to throw away the *Fake* dimensions and keep only the *Rank* dimensions (the true signals).

Thus, 

|Concept|Definition|Intuition|
|---|---|---|
|**Columns**|The vectors $v_1, v_2, \dots, v_n$ that make up the matrix $A$.|The Raw tools, Arrows, some of which may be redundant|
|**Span**|The set of all possible linear combinations of a list of vectors: $S = \{ c_1v_1 + \dots + c_nv_n \}$.|The Cloud. The total shape created by stretching and combining the raw tools in every possible way|
|**Range (Column Space)**|The subspace of outputs reachable by the linear transformation $f(x) = Ax$. Mathematically equivalent to the Span of the columns.|The Reach. When we view the matrix as a machine, the Range is the specific "territory" the machine can touch.|
|**Basis**|A minimal set of linearly independent vectors that spans a subspace.|The Skeleton. If you strip away all the redundant columns (the fake tools), this is the clean, efficient set of arrows left over that still builds the same Cloud.|
|**Rank**|The dimension of the Column Space.|The Score. A single number representing the "True Dimension" of the output. It tells how many useful dimensions exist in your data.|

> The **Columns** of the matrix generate a **Span**. When viewed as a function, this Span is called the **Range**. The smallest set of vectors needed to describe this Range is the **Basis**, and the count of vectors in that Basis is the **Rank**.

## Space Warper

Modify the transformation matrix by dragging the basis vectors ($\hat{i}$ and $\hat{j}$) or by changing the sliders values representing :

$$ \text{Transformation Matrix} = \begin{bmatrix} a&b \\ c&d\end{bmatrix} = \begin{bmatrix} 1&0 \\ 0&1 \end{bmatrix} $$

- Vector $\begin{bmatrix} a\\c \end{bmatrix}$ represents $\hat{i}$.
- Vector $\begin{bmatrix} b\\d \end{bmatrix}$ represents $\hat{j}$.

When the <span class="green">green</span> arrow aligns with <span class="red">red</span> arrow, it signifies a dimension loss.

<wasm-sim src="space_warper">
        <script type="application/json">
        [
            {"id": "ix", "label": "Matrix a (i.x)", "min": -3, "max": 3, "val": 1, "step": 0.1},
            {"id": "iy", "label": "Matrix c (i.y)", "min": -3, "max": 3, "val": 0, "step": 0.1},
            {"id": "jx", "label": "Matrix b (j.x)", "min": -3, "max": 3, "val": 0, "step": 0.1},
            {"id": "jy", "label": "Matrix d (j.y)", "min": -3, "max": 3, "val": 1, "step": 0.1},
            {"id": "reset", "label": "Reset Matrix", "type": "button"}
        ]
        </script>
</wasm-sim>


---

With this this post on matrices and their geometric implementation, types of matrices and different operations using matrices is completed.

<script src="static/js/wasm_engine.js"></script>