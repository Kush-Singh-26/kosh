---
title: "6. Maths4ML: Matrix Decomposition & SVD"
date: "2026-01-03"
description: "The Art of Mathematical Forgetting: How to throw away 90% of your data without losing the meaning."
tags: ["Maths for ML"]
pinned: false
---

## Matrix Decomposition : Prime Factorization of Matrices

The process of breaking down a complex matrix into a product of simpler matrices.

$$\underbrace{A}_{\text{Complex Data}} = \underbrace{B}_{\text{Simple}} \times \underbrace{C}_{\text{Simple}}$$

- It is computationally efficient to perform operations like matrix inversion on triangular or diagonal matrices rather than dense matrix.
- Decomposition reveals hidden (latent) structures in the data that are not visible in the raw matrix.

There are many types of decompositions. Some of the  fundamental decompositions are :
1. LU Decomposition
2. QR Decomposition
3. Eigen Decomposition
    - Already covered in [previous blog](./Maths-determinant-eigenvectors.md).
4. SVD

---

## 1. LU Decomposition

$$ A = L \cdot U $$

It splits a square matrix $A$ into 2 **triangular matrices** $L$ ans $U$. It is like Gaussian Elimination with a memory.
- $L$ : Lower Triangular matrix. It represents the record of the row operations (multipliers) used during elimination.
- $U$ : Upper Triangular matrix. This is the Row Echelon Form, the final result of the elimination process.

### Example 

$$ A = \begin{bmatrix} 4&3 \\ 6&3 \end{bmatrix} $$

Goal is to find $L$ & $U$ such that $LU=A$.

- To make $U$ upper trainagular (gaussian elimination) : $R_2 \rightarrow R_2 - 1.5 \times R_1$.
- $R_2 = [0, -1.5]$.

$$ U = \begin{bmatrix} 4&3 \\ 0&-1.5 \end{bmatrix} $$

- $L$ is a lower triangular matrix. Its diagonal will always be one. 
- If the row operation $R_i \rightarrow R_i - \ell_{ij} \times R_j$ to create a zero at position $(i, j)$ in $U$, then the entry in the $L$ matrix at position $(i, j)$ is exactly $\ell_{ij}$.

Thus,

$$L = \begin{bmatrix} 1 & 0 \\ 1.5 & 1 \end{bmatrix}$$

To verify,

$$LU = \begin{bmatrix} 1 & 0 \\ 1.5 & 1 \end{bmatrix} \begin{bmatrix} 4 & 3 \\ 0 & -1.5 \end{bmatrix} = \begin{bmatrix} (1)(4)+0 & (1)(3)+0 \\ (1.5)(4)+0 & (1.5)(3) + (1)(-1.5) \end{bmatrix} = \begin{bmatrix} 4 & 3 \\ 6 & 3 \end{bmatrix} = A$$

### Use of LU Decomposition

Solving a system $A\mathbf{x} = \mathbf{b}$ using $A^{-1}$ is computationally expensive ($O(N^3)$) and unstable. With $A=LU$, it is solvable in two fast $O(N^2)$ steps:
1. **Forward Substitution** : Solve $L\mathbf{y} = \mathbf{b}$ for $\mathbf{y}$.
2. **Backward Substitution** : Solve $U\mathbf{x} = \mathbf{y}$ for $\mathbf{x}$.

> It is used in almost all linear algebra libraries like `NumPy`

---

## 2. QR Decomposition

$$ A = Q \cdot R $$

It decomposes the matrix $A$ into :
- $Q$ : **Orthogonal Matrix** whose columns are orthonormal.
- $R$ : **Upper Triangle Matrix**

---

1. **Orthogonal** :
- Two vectors are orthogonal if they are at a $90 \degree$ angle to each other.
    - Their dot product is zero. ($u \cdot v = 0$).

2. **Orthonormal** :
- Two vectors are orthonormal if they are perpendicular and they both have a length of exactly 1.
    - Their dot product is zero. ($u \cdot v = 0$).
    - Length of each vector is 1 (6$||u|| = 1$ and 7$||v|| = 1$). 

**Normalization** : Turning an Orthogonal set into an Orthonormal set by dividing each vector by its own length.

Another important fact is : $Q^\top Q = I$. This means $Q^{-1} = Q^\top$

---

### Example

Using **Gram-Schmidt process** to  decompose this matrix :

$$A = \begin{bmatrix} 1 & 1 \\ 1 & 0 \end{bmatrix}$$

Columns vectors of $A$ : $a_1 = \begin{bmatrix} 1 \\ 1 \end{bmatrix}$ and $a_2 = \begin{bmatrix} 1 \\ 0 \end{bmatrix}$.

> **Goal** : - Turn $a_1$ and $a_2$ into orthonormal vectors $e_1$ and $e_2$.

1. Find $e_1$ : 
- $||a_1|| = \sqrt{1^2 + 1^2} = \sqrt{2}$.
- $e_1 = \frac{a_1}{||a_1||} = \begin{bmatrix} 1/\sqrt{2} \\ 1/\sqrt{2} \end{bmatrix}$.

2. Make $a_2$ orthogonal to $e_1$, then normalize to find $e_2$ :
-  find the projection of $a_2$ onto $e_1$: $(a_2 \cdot e_1)e_1$ :

$$\underbrace{\text{Projection Vector}}_{\text{Vector}} = \underbrace{(a_2 \cdot e_1)}_{\text{Length (Scalar)}} \times \underbrace{e_1}_{\text{Direction (Unit Vector)}}$$

- $a_2 \cdot e_1 = (1)(1/\sqrt{2}) + (0)(1/\sqrt{2}) = 1/\sqrt{2}$
- Subtract the projection from $a_2$ to get orthogonal part $u_2$ :

$$u_2 = a_2 - (1/\sqrt{2})e_1 = \begin{bmatrix} 1 \\ 0 \end{bmatrix} - \begin{bmatrix} 0.5 \\ 0.5 \end{bmatrix} = \begin{bmatrix} 0.5 \\ -0.5 \end{bmatrix}$$

- Normalize $u_2$ to get $e_2$. ($||u_2|| = \sqrt{0.5^2 + (-0.5)^2} = \sqrt{0.5} = 1/\sqrt{2}$)
- $e_2 = \frac{u_2}{1/\sqrt{2}} = \sqrt{2} \begin{bmatrix} 0.5 \\ -0.5 \end{bmatrix} = \begin{bmatrix} 1/\sqrt{2} \\ -1/\sqrt{2} \end{bmatrix}$.

Thus,

$$Q = \begin{bmatrix} 1/\sqrt{2} & 1/\sqrt{2} \\ 1/\sqrt{2} & -1/\sqrt{2} \end{bmatrix}$$

Finding $R$ : $R$ contains the dot products (projections) calculated during the Gram-Schmidt process.

$$R = \begin{bmatrix} a_1 \cdot e_1 & a_2 \cdot e_1 \\ 0 & a_2 \cdot e_2 \end{bmatrix}$$

$$R = \begin{bmatrix} \sqrt{2} & 1/\sqrt{2} \\ 0 & 1/\sqrt{2} \end{bmatrix}$$

### Use of QR Decomposition

- It provide numerical stability as it doesn't amplify errors.

#### Least Square Problem

The problem is :

$$ \argmin_x \| Ax-b \|_2 $$

> Find the value of $x$ that minimizes the squared Euclidean distance between $Ax$ and $b$.

As discussed above, QR decomposition will give $A = QR$. Thus,

$$ QRx = b $$

Multiplying both sides by $Q^\top$ :

$$Q^T (QRx) = Q^T b$$

Because  $Q^T Q = I$ 

$$Rx = Q^T b$$

It is much faster to multiply with an Upper Triangular Matrix.


> QR decomposition takes skewed, correlated vectors ($A$) and mathematically "straightens" them into a perfectly perpendicular grid ($Q$), while recording the original "lean" or correlation in the triangular matrix $R$.

---

To summarize, 

|Category| Goal|Key Examples|
|---|---|---|
|Solving Systems|Break $A$ into triangular forms to solve $A\mathbf{x}=\mathbf{b}$ faster.|LU|
|Orthogonalizing|Isolate the direction of vectors from their scaling/shear.|QR Decomposition|
|Spectral Analysis|Find the axes along which the matrix just "stretches" space.|Eigendecomposition (for square matrix) , SVD|

---

## Singular Value Decomposition (SVD)

While Eigenvectors identify lines that stay parallel during a transformation, SVD generalizes this concept to all matrices, even rectangular ones. It reveals the fundamental geometry of data transformations by breaking a complex operation into three distinct, elementary actions.

The core principle behind SVD is :
> Every linear transformation, no matter how complex, is actually just a sequence of three simple moves.

For any real matrix $A$ of size $m \times n$ (where $m$ is the number of rows/samples and $n$ is the number of columns/features), the decomposition is:

$$ A = U \Sigma V^\top $$

It represents : $\text{Rotate} \rightarrow \text{Stretch} \rightarrow \text{Rotate}$. To understand the equation, it must be read from right to left as applied to a vector $x$ in $Ax$ :

1. $V^\top$: Rotation/Reflection in the Input Space ($n$-dimensional).
2. $\Sigma$: Scaling/Stretching along the axes.
3. $U$: Rotation/Reflection in the Output Space ($m$-dimensional).

### 1. **$V^\top$ (First Rotation)**

The transpose of the Right Singular Matrix. 
- It is an $n \times n$ orthogonal matrix.
- It takes the input vectors and rotates them to align with the "natural axes" of the data. 
- It does not change the length of vectors, only their orientation.
- The rows of $V^\top$ (which are the columns of $V$) are the eigenvectors of $A^\top A$.
    - $v_1$ (the first row of $V^\top$) aligns with the direction of maximum variance (the "spine") of the data.
    - $v_2$ aligns with the second greatest variance, perpendicular to $v_1$.
- This acts as a "change of basis" in the input domain.

### 2. **$\Sigma$ (The Stretch)**

The Singular Value Matrix.
- It is an $m \times n$ diagonal matrix (mostly zeros, with positive numbers on the main diagonal).
- It stretches or shrinks the space along the axes defined by $V$.
    - It doesn't rotate or shear as it is a diagonal matrix.
- The diagonal entries are : $\sigma_1, \sigma_2, \dots, \sigma_r$. These are called **Singular Values**.
    - They are always real and non-negative, sorted in descending order ($\sigma_1 \geq \sigma_2 \geq \dots \geq 0$).
    - The value of $\sigma_i$ dictates the "strength" or "energy" of the transformation along that axis. 
        - A singular value of 0 indicates that dimension is squashed into nothingness (loss of information).
- $\sigma_i = \sqrt{\lambda_i}$, where $\lambda_i$ are the eigenvalues of $A^\top A$.

### 3. **$U$ (The Final Rotation)**

The Left Singular Matrix.
- It is an $m \times m$ orthogonal matrix.
-  After the input has been rotated ($V^\top$) and stretched ($\Sigma$), it now lives in the output dimensions. $U$ rotates this result to align it with the standard axes of the output space.
- The columns of $U$ are the eigenvectors of $AA^\top$.
    - These vectors usually represent some patterns in the output space.

|Component|Matrix Shape|Type|Represents|Derived From|
|---|---|---|---|---|
|$U$|$m \times m$|Orthogonal|Output Space Directions|Eigenvectors of $AA^\top$|
|$\Sigma$|$m \times n$|Diagonal|Stretching Factors (Gain)|$\sqrt{\text{Eigenvalues of } A^\top A}$|
|$V^\top$|$n \times n$|Orthogonal|Input Space Directions|Eigenvectors of $A^\top A$|

> -  It is called Singular Value Decomposition because it factorizes a matrix specifically to expose the critical Singular Values, the numbers that tell you if and how the matrix collapses space (becomes singular).
> - Geometrically, a Singular Matrix "crushes" space (e.g., flattens a 3D cube into a 2D sheet or 1D line).

---

### Why $A^\top A$ and $AA^\top$?

These are co-variance **matrices**. If $A = U\Sigma V^\top$, then $A^\top A$ : 

$$A^\top A = (U \Sigma V^\top)^\top (U \Sigma V^\top)$$

$$A^\top A = V \Sigma^\top U^\top U \Sigma V^\top$$

Since $U$ is orthogonal, $U^\top U = I$

$$A^\top A = V (\Sigma^\top \Sigma) V^\top$$

This is exactly like the **Eigendecomposition formula** ($PD^{-1}P$).
- $V$: The eigenvector matrix of $A^\top A$.
- $\Sigma^\top \Sigma$: The eigenvalue matrix of $A^\top A$. 
    - Because $\Sigma$ is diagonal, $\Sigma^\top \Sigma$ just contains the squared singular values ($\sigma^2$) on the diagonal.

---

## Solved Example of SVD

$$A = \begin{bmatrix} 3 & 2 & 2 \\ 2 & 3 & -2 \end{bmatrix}$$

- **Goal** : Find $U$ ($2\times2$), $\Sigma$ ($2\times3$), and $V$ ($3\times3$).

### 1. Compute $A^\top A$ :

$$ A A^T = \begin{bmatrix} 3 & 2 & 2 \\ 2 & 3 & -2 \end{bmatrix} \begin{bmatrix} 3 & 2 \\ 2 & 3 \\ 2 & -2 \end{bmatrix} = \begin{bmatrix} 17 & 8 \\ 8 & 17 \end{bmatrix} $$

### 2. Find the eigenvectors of this symmetric matrix : 

Solve $\det(A A^T - \lambda I) = 0$.

$$(17 - \lambda)^2 - 64 = 0 \implies 17 - \lambda = \pm 8$$

- This gives $\lambda_1 = 25$ and $\lambda_2 = 9$

### 3. Find Singular Values ($\Sigma$): 

Square root of the eigen values :

$\sigma_1 = \sqrt{25} = 5$ and $\sigma_2 = \sqrt{9} = 3$

### 4. Find Eigenvectors (Columns of $U$)

Solving the linear system $(A - \lambda I)u = 0$ for the vector $u = \begin{bmatrix} x \\ y \end{bmatrix}$ will give the eigen vectors.

- For $\lambda_1 = 25$:

$$\begin{bmatrix} 17-25 & 8 \\ 8 & 17-25 \end{bmatrix}\vec{u} = \begin{bmatrix} -8 & 8 \\ 8 & -8 \end{bmatrix}\vec{u} = 0 \implies u_1 = \begin{bmatrix} 1 \\ 1 \end{bmatrix}$$

- Normalize (divide by length $\sqrt{2}$ (magnitude of $u_1$)) : $u_1 = \begin{bmatrix} 1/\sqrt{2} \\ 1/\sqrt{2} \end{bmatrix}$

- For $\lambda_2 = 9$:

$$\begin{bmatrix} 8 & 8 \\ 8 & 8 \end{bmatrix}\vec{u} = 0 \implies u_2 = \begin{bmatrix} -1 \\ 1 \end{bmatrix}$$

- Normalize: $u_2 = \begin{bmatrix} -1/\sqrt{2} \\ 1/\sqrt{2} \end{bmatrix}$

Thus, 

$$U = \begin{bmatrix} \frac{1}{\sqrt{2}} & -\frac{1}{\sqrt{2}} \\ \frac{1}{\sqrt{2}} & \frac{1}{\sqrt{2}} \end{bmatrix}$$

### 5. Construct $\Sigma$ (The Stretch)

$\Sigma$ must have the same dimensions as the original matrix $A$ ($2 \times 3$). Place the singular values on the diagonal and pad the rest with zeros.

$$\Sigma = \begin{bmatrix} 5 & 0 & 0 \\ 0 & 3 & 0 \end{bmatrix}$$

### 6. Find $V$ (Right Singular Vectors)

Using relation $A^T u = \sigma v \rightarrow v = \frac{1}{\sigma} A^T u$.

<details>
<summary>How did this relation came about ?</summary>

$$A^T = (U \Sigma V^T)^T$$

$$A^T = (V^T)^T \Sigma^T U^T$$

$$A^T = V \Sigma U^T$$

> $\Sigma$ is diagonal, so $\Sigma^T = \Sigma$.
 
Multiply both sides by a vector $u$, one of the columns of $U$ : $u_i$.

>$U^T u_i$ will result in a vector of zeros with a single $1$ at index $i$, because $U$ is orthonormal.

$$A^T u_i = V \Sigma (U^T u_i)$$

$$A^T u_i = \sigma_i v_i$$

</details>

- Calculate $v_1$ (using $\sigma_1 = 5, u_1$):

$$v_1 = \frac{1}{5} \begin{bmatrix} 3 & 2 \\ 2 & 3 \\ 2 & -2 \end{bmatrix} \begin{bmatrix} \frac{1}{\sqrt{2}} \\ \frac{1}{\sqrt{2}} \end{bmatrix} = \frac{1}{5\sqrt{2}} \begin{bmatrix} 5 \\ 5 \\ 0 \end{bmatrix} = \begin{bmatrix} \frac{1}{\sqrt{2}} \\ \frac{1}{\sqrt{2}} \\ 0 \end{bmatrix}$$

- Similarly,

$$v_2 =  \begin{bmatrix} \frac{-1}{3\sqrt{2}} \\ \frac{1}{3\sqrt{2}} \\ \frac{-4}{3\sqrt{2}} \end{bmatrix}$$

- Because, $\sigma_3 = 0$, so not posiible to use above formula. But since, $V$ is orthogonal matrix, hence all its vectors (columns) are perpendicular. Thus using cross product, 

$$v_3 = v_1 \times v_2 = \begin{bmatrix} -2/3 \\ 2/3 \\ 1/3 \end{bmatrix}$$

> The vector orthogonal to $(1, 1, 0)$ and $(-1, 1, -4)$ is $(-4, 4, 2)$. Normalized by 6.

Finally, 

$$V = \begin{bmatrix} \frac{1}{\sqrt{2}} & \frac{-1}{3\sqrt{2}} & \frac{-2}{3} \\ \frac{1}{\sqrt{2}} & \frac{1}{3\sqrt{2}} & \frac{2}{3} \\ 0 & \frac{-4}{3\sqrt{2}} & \frac{1}{3} \end{bmatrix}$$

---

Thus, reconstruct $A$ using $U \Sigma V^T$ :

$$A = \underbrace{\begin{bmatrix} \frac{1}{\sqrt{2}} & \frac{-1}{\sqrt{2}} \\ \frac{1}{\sqrt{2}} & \frac{1}{\sqrt{2}} \end{bmatrix}}_{U (2 \times 2)} \cdot \underbrace{\begin{bmatrix} 5 & 0 & 0 \\ 0 & 3 & 0 \end{bmatrix}}_{\Sigma (2 \times 3)} \cdot \underbrace{\begin{bmatrix} \frac{1}{\sqrt{2}} & \frac{1}{\sqrt{2}} & 0 \\ \frac{-1}{3\sqrt{2}} & \frac{1}{3\sqrt{2}} & \frac{-4}{3\sqrt{2}} \\ \frac{-2}{3} & \frac{2}{3} & \frac{1}{3} \end{bmatrix}}_{V^T (3 \times 3)}$$


>1. $V^T$ takes a 3D vector and rotates it in 3D.
>2. $\Sigma$ takes that 3D vector and removes the last dimension (multiplying by 0), landing back in 2D space.
>3. $U$ rotates that 2D result.

---

## SVD Interactive Explorer

Using this simulation all the above theory about SVD can be visualized.

Use the presets to see how different linear transformations are decomposed into Rotation $\rightarrow$ Stretch $\rightarrow$ Rotation.

<wasm-sim src="svd_sim">
        <script type="application/json">
        [
            {"id": "t", "label": "Animation Sequence", "min": 0, "max": 3, "val": 0, "step": 0.02},
            {"id": "a", "label": "Matrix a", "min": -2, "max": 2, "val": 1, "step": 0.1},
            {"id": "b", "label": "Matrix b", "min": -2, "max": 2, "val": 0.5, "step": 0.1},
            {"id": "c", "label": "Matrix c", "min": -2, "max": 2, "val": 0, "step": 0.1},
            {"id": "d", "label": "Matrix d", "min": -2, "max": 2, "val": 1, "step": 0.1},
            {"id": "preset_identity", "label": "Preset: Identity", "type": "button"},
            {"id": "preset_shear", "label": "Preset: Shear", "type": "button"},
            {"id": "preset_rotate", "label": "Preset: Rotation (45\u00B0)", "type": "button"},
            {"id": "preset_reflection", "label": "Preset: Reflection (Flip X)", "type": "button"},
            {"id": "preset_singular", "label": "Preset: Singular (Collapse)", "type": "button"},
            {"id": "regenerate", "label": "Regenerate Cloud", "type": "button"}
        ]
        </script>
</wasm-sim>

---

## Matrix as a Sum of Layers

> Instead of a giant wall of numbers, matrices can be visualized as a stack of simple, transparent sheet

### Outer Product

- **Dot Product (Inner Product)** : Takes two vectors and squashes them into a single number (a scalar).
- **Outer Product** : Takes two vectors and explodes them into a matrix.

> A column vector $u$ and a row vector $v^T$ when multiplied will create a grid (a matrix) where every row is just a copy of $v$ scaled by a number from $u$.

The matrix created this way $uv^\top$ has **Rank 1**. Thus, it is the simplest possible matrix. So, all the rows and columns are parallel to each other. **One simple pattern is repeated across the grid.**

---

$$ A = U\Sigma V^\top $$

- $U$ is a matrix of columns or eigenvectors of $AA^\top$:

$$\begin{bmatrix} | & | & \\ u_1 & u_2 & \dots \\ | & | & \end{bmatrix}$$

- $\Sigma$ is a diagonal matrix: 

$$\begin{bmatrix} \sigma_1 & 0 & \dots \\ 0 & \sigma_2 & \dots \\ \vdots & \vdots & \ddots \end{bmatrix}$$

- $V^T$ is a matrix of rows (eigenvectors of $A^\top A$): 

$$\begin{bmatrix} - & v_1^T & - \\ - & v_2^T & - \\ & \vdots & \end{bmatrix}$$

1. Multiply $\Sigma V^\top$

$$\Sigma V^T = \begin{bmatrix} \sigma_1 v_1^T \\ \sigma_2 v_2^T \\ \vdots \\ \sigma_r v_r^T \end{bmatrix}$$

So, the equation becomes $A = U \times (\Sigma V^T)$.

2. Multiplying the Coumn $\times$ Row :

- multiply $U$ (columns) by the result from Step 1 (rows).

$$\begin{bmatrix} | & | \\ u_1 & u_2 \\ | & | \end{bmatrix} \times \begin{bmatrix} - & r_1 & - \\ - & r_2 & - \end{bmatrix} = (u_1 \times r_1) + (u_2 \times r_2)$$

Thus applying this to the SVD matrices :

- Column 1 of $U$ is $u_1$.
- Row 1 of $(\Sigma V^T)$ is $\sigma_1 v_1^T$.

Their product gives : $\sigma_1 u_1 v_1^T$.

Similarly for all other rows and columns :

$$A = \underbrace{\sigma_1 u_1 v_1^T}_{\text{Layer 1}} + \underbrace{\sigma_2 u_2 v_2^T}_{\text{Layer 2}} + \dots$$

$$A = \sum_{i} \sigma_i u_i v_i^T$$

- $u_1 v_1^T$ is the Pattern.
- $\sigma_1$ is the Importance / opacity of the pattern in the final data.

> $\sigma_i$ s are always in descending order.

So, Layer 1 will capture the highest information, and the importance of each later will decrease subsequently.

Summing up all these layers will give the perfect high-resolution image.

## Use of SVD (Image Compression)

Because SVD sorts the layers by importance (from largest $\sigma$ to smallest), we know that the bottom layers contribute almost nothing to the visible image.

So, deleting the bottom 50 layers (set $\sigma_{51} \dots \sigma_{100}$ to 0), we save a huge amount of storage space, having very minimal effect on the final appearance. Lose the noise and retain the signals.

> SVD proves that a complex dataset is just a sum of simple patterns, ordered by how "loud" they are. You can mute the quiet ones to compress the data without losing the meaning.

## Eigenface Compression

To prove that quite patterns can be muted without losing the meaning, let's look at a generated face. A face is highly structured—two eyes, a nose, and a mouth are always in roughly the same place. SVD exploits this structure to compress the image massively.

>Drag the slider from **k=1** up to **k=50**.
>
> * **Rank 1 (The Ghost):** The single "loudest" pattern. It captures the average head shape and lighting direction. It looks like a blurred mask.
> * **Rank 10 (The Identity):** By adding just 9 more layers, the eyes, nose bridge, and mouth become sharp. It is now possible to recognize the person.
> * **Rank 50 (The Texture):** The final layers add the "quiet" details—skin texture, noise, and subtle imperfections.

<wasm-sim src="svd_photo">
        <script type="application/json">
        [
            {"id": "k", "label": "Rank k (Eigenvectors Used)", "min": 1, "max": 50, "val": 1, "step": 1}
        ]
        </script>
</wasm-sim>

**The Result:** Even at Rank 10, the image is recognizable, yet we have discarded over 80% of the raw data. This technique (**Eigenfaces**) was the foundation of early facial recognition systems—reducing complex human faces to a simple list of 10-20 numbers.

---

With this, the post on different types of matrix decomposition and their uses, then SVD and how it works is completed.

<script src="static/js/wasm_engine.js"></script>